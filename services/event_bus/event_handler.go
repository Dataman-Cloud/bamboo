package event_bus

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/QubitProducts/bamboo/configuration"
	"github.com/QubitProducts/bamboo/services/application"
	"github.com/QubitProducts/bamboo/services/haproxy"
	"github.com/QubitProducts/bamboo/services/service"
	"github.com/QubitProducts/bamboo/services/template"
)

var TemplateInvalid bool

type MarathonEvent struct {
	// EventType can be
	// api_post_event, status_update_event, subscribe_event
	EventType string
	AppId     string
	Timestamp string
}

type ApiPostEvent struct {
	EventType     string
	TimeStamp     string
	AppDefinition AppDefinition
}

type AppDefinition struct {
	Id string
}

type StatusUpdateEvent struct {
	EventType string
	TimeStamp string
	AppId     string
}

type ZookeeperEvent struct {
	Source    string
	EventType string
}

type ServiceEvent struct {
	EventType string
}

type WeightEvent struct {
	EventType string
}

type Handlers struct {
	Conf       *configuration.Configuration
	Storage    service.Storage
	AppStorage application.Storage
	AppId      string
}

func (h *Handlers) MarathonEventHandler(event MarathonEvent) {
	if event.EventType == "event_stream_attached" && event.AppId != "" {
		h.AppId = strings.TrimLeft(event.AppId, "/")
		log.Printf("%s-----> %s => %s\n", event.AppId, event.EventType, event.Timestamp)
		queueUpdate(h)
		h.Conf.StatsD.Increment(1.0, "callback.marathon", 1)
	}
}

func (h *Handlers) ServiceEventHandler(event ServiceEvent) {
	log.Println("Domain mapping: Stated changed")
	queueUpdate(h)
	h.Conf.StatsD.Increment(1.0, "reload.domain", 1)
}

func (h *Handlers) WeightEventHandler(event WeightEvent) {
	log.Println("Weight changed")
	frontendMapJson, _ := json.Marshal(haproxy.FrontendMap)
	log.Println("frontendMap", string(frontendMapJson))

	weights, err := h.AppStorage.All()
	if err != nil {
		log.Println("Error: can't fetch weights", err.Error())
		return
	}
	weightJson, _ := json.Marshal(weights)
	log.Println("weight", string(weightJson))

	for _, weight := range weights {
		if frontend, ok := haproxy.FrontendMap[weight.ID]; ok {
			servers := haproxy.CalcWeights(frontend, weight)
			updateWeight(h.Conf, servers)
		}
	}
	// save weight into config file for haproxy recovery
	content, err := generateConfig(h)
	if err != nil {
		log.Println("can't generate config", err.Error())
	}
	err = ioutil.WriteFile(h.Conf.HAProxy.OutputPath, []byte(content), 0666)
	if err != nil {
		log.Println("Failed to write template on path", h.Conf.HAProxy.OutputPath)
	}
}

func updateWeight(conf *configuration.Configuration, servers []map[string]interface{}) {
	if len(servers) < 1 {
		log.Println("empty servers")
		return
	}

	json, err := json.Marshal(servers)
	if err != nil {
		log.Println(err.Error())
		log.Println("Error: can't update app weight")
		return
	}
	log.Println("serversJson", string(json))

	client := &http.Client{}
	addr := fmt.Sprintf("%s:%s/api/weight", conf.HAProxy.IP, conf.HAProxy.Port)
	req, err := http.NewRequest("PUT", addr, bytes.NewBuffer(json))
	req.Close = true
	if err != nil {
		log.Println("Failed to creat new http request: ", err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Http request failed: ", err)
		return
	}
	defer resp.Body.Close()

	log.Println("updated", string(json), resp.StatusCode)
}

var updateChan = make(chan *Handlers, 1)

func init() {
	go func() {
		log.Println("Starting update loop")
		for {
			h := <-updateChan
			handleHAPUpdate(h)
		}
	}()
}

var queueUpdateSem = make(chan int, 1)

func queueUpdate(h *Handlers) {
	queueUpdateSem <- 1

	select {
	case _ = <-updateChan:
		log.Println("Found pending update request. Don't start another one.")
	default:
		log.Println("Queuing an haproxy update.")
	}
	updateChan <- h

	<-queueUpdateSem
}

func handleHAPUpdate(h *Handlers) {
	reloadStart := time.Now()
	// TODO fetch app from marathon and register it to F5
	reloaded, err := ensureLatestConfig(h)

	if err != nil {
		h.Conf.StatsD.Increment(1.0, "haproxy.reload.error", 1)
		log.Println("Failed to update HAProxy configuration:", err)
	} else if reloaded {
		h.Conf.StatsD.Timing(1.0, "haproxy.reload.marathon.duration", time.Since(reloadStart))
		h.Conf.StatsD.Increment(1.0, "haproxy.reload.marathon.reloaded", 1)
		log.Println("Reloaded HAProxy configuration")
	} else {
		h.Conf.StatsD.Increment(1.0, "haproxy.reload.skipped", 1)
		log.Println("Skipped HAProxy configuration reload due to lack of changes")
	}
}

// For values of 'latest' conforming to general relativity.
func ensureLatestConfig(h *Handlers) (reloaded bool, err error) {
	content, err := generateConfig(h)
	if err != nil {
		return
	}

	req, err := isReloadRequired(h.Conf.HAProxy.OutputPath, content)
	if err != nil || !req {
		return
	}

	/*	err = validateConfig(conf.HAProxy.ReloadValidationCommand, content)
		if err != nil {
			return
		}*/

	defer cleanupConfig(h.Conf.HAProxy.ReloadCleanupCommand)

	reloaded, err = changeConfig(h.Conf, content)
	if err != nil {
		return
	}

	return
}

// Generates the new config to be written
func generateConfig(h *Handlers) (config string, err error) {
	conf := h.Conf
	templateContent, err := ioutil.ReadFile(conf.HAProxy.TemplatePath)
	if err != nil {
		log.Println("Failed to read template contents")
		return
	}

	templateData, err := haproxy.GetTemplateData(conf, h.Storage, h.AppStorage)
	if err != nil {
		log.Println("Failed to retrieve template data")
		TemplateInvalid = true
		return
	}

	config, err = template.RenderTemplate(conf.HAProxy.TemplatePath, string(templateContent), templateData)
	if err != nil {
		log.Println("Template syntax error")
		TemplateInvalid = true
		return
	}
	TemplateInvalid = false
	return
}

// Loads the existing config and decides if a reload is required
func isReloadRequired(configPath string, newContent string) (bool, error) {
	// An error here means that the template may not exist, in which case we simply continue
	currentContent, err := ioutil.ReadFile(configPath)

	if err == nil {
		return newContent != string(currentContent), nil
	} else if os.IsNotExist(err) {
		return true, nil
	}

	return false, err // Returning false here as is default value for bool
}

// Takes the ReloadValidateCommand and returns nil if the command succeeded
func validateConfig(validateTemplate string, newContent string) (err error) {
	if validateTemplate == "" {
		return nil
	}

	tmpFile, err := ioutil.TempFile("/tmp", "bamboo")
	if err != nil {
		return
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	log.Println("Generating validation command")
	_, err = tmpFile.WriteString(newContent)
	if err != nil {
		return
	}

	validateCommand, err := template.RenderTemplate(
		"validate",
		validateTemplate,
		tmpFile.Name())
	if err != nil {
		return
	}

	log.Println("Validating config")
	err = execCommand(validateCommand)

	return
}

func changeConfig(conf *configuration.Configuration, newContent string) (reloaded bool, err error) {
	// This failing scares me a lot, as could end up with very invalid config
	// content. I'd suggest restoring the original config, but that adds all
	// kinds of new and interesting failure cases
	log.Println("Change Config")
	err = ioutil.WriteFile(conf.HAProxy.OutputPath, []byte(newContent), 0666)
	if err != nil {
		log.Println("Failed to write template on path", conf.HAProxy.OutputPath)
		return
	}

	/*	err = execCommand(conf.HAProxy.ReloadCommand)
		if err != nil {
			return
		}*/

	client := &http.Client{}
	addr := fmt.Sprintf("%s:%s/api/haproxy", conf.HAProxy.IP, conf.HAProxy.Port)
	req, err := http.NewRequest("PUT", addr, nil)
	if err != nil {
		log.Println("Failed to creat new http request: ", err)
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Println("Http request failed: ", err)
		return
	}
	defer resp.Body.Close()

	reloaded = true
	return
}

// This will be executed in a deferred, so is rather self contained
func cleanupConfig(command string) {
	log.Println("Cleaning up config")
	execCommand(command)
}

func execCommand(cmd string) error {
	log.Printf("Exec cmd: %s \n", cmd)
	output, err := exec.Command("sh", "-c", cmd).CombinedOutput()
	if err != nil {
		log.Println(err.Error())
		log.Println("Output:\n" + string(output[:]))
	}
	return err
}
