package event_bus

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"

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

type ServiceEvent struct {
	EventType string
}

type Handlers struct {
	Conf       *configuration.Configuration
	Storage    service.Storage
	AppStorage application.Storage
}

func (h *Handlers) MarathonEventHandler(event MarathonEvent) {
	if h.Conf.Application.Id == strings.TrimLeft(event.AppId, "/") || event.EventType == "event_stream_attached" {
		log.Printf("%s-----> %s => %s\n", event.AppId, event.EventType, event.Timestamp)
		queueUpdate(h)
	}
}

func (h *Handlers) ServiceEventHandler(event ServiceEvent) {
	log.Println("app status changed")
	queueUpdate(h)
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
	reloaded, err := ensureLatestConfig(h)
	if err != nil {
		log.Println("Failed to update HAProxy configuration:", err)
	}
	if reloaded {
		log.Println("The HAProxy configuration has been reloaded")
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

	reloaded, err = changeConfig(h.Conf, content)
	if err != nil {
		return
	}

	return
}

// Generates the new config to be written
func generateConfig(h *Handlers) (config string, err error) {
	templateContent, err := ioutil.ReadFile(h.Conf.HAProxy.TemplatePath)
	if err != nil {
		log.Println("Failed to read template contents")
		return
	}

	templateData, err := haproxy.GetTemplateData(h.Conf)
	if err != nil {
		log.Println("Failed to retrieve template data")
		TemplateInvalid = true
		return
	}

	config, err = template.RenderTemplate(h.Conf.HAProxy.TemplatePath, string(templateContent), templateData)
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
