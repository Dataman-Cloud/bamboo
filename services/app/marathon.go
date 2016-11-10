package app

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QubitProducts/bamboo/configuration"
)

// Describes an app process running
type Task struct {
	Frontend string
	Server   string
	Host     string
	Port     int
	Ports    []int
	Version  string
	Weight   int
}

// A health check on the application
type HealthCheck struct {
	// One of TCP, HTTP or COMMAND
	Protocol string
	// The path (if Protocol is HTTP)
	Path string
	// The position of the port targeted in the ports array
	PortIndex int
}

type Endpoint struct {
	Protocol string
	Bind     int
}

// An app may have multiple processes
type App struct {
	Id              string
	Frontend        string
	HealthCheckPath string
	HealthChecks    []HealthCheck
	Tasks           []Task
	ServicePort     int
	ServicePorts    []int
	Env             map[string]string
	Labels          map[string]string
	Endpoints       []Endpoint
	CurVsn          string
}

type AppList []App

func (slice AppList) Len() int {
	return len(slice)
}

func (slice AppList) Less(i, j int) bool {
	return slice[i].Id < slice[j].Id
}

func (slice AppList) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type marathonTaskList []marathonTask

type marathonTasks struct {
	Tasks marathonTaskList `json:"tasks"`
}

type marathonTask struct {
	AppId        string
	Id           string
	Host         string
	Ports        []int
	ServicePorts []int
	StartedAt    string
	StagedAt     string
	Version      string
}

func (slice marathonTaskList) Len() int {
	return len(slice)
}

func (slice marathonTaskList) Less(i, j int) bool {
	return slice[i].Id < slice[j].Id
}

func (slice marathonTaskList) Swap(i, j int) {
	slice[i], slice[j] = slice[j], slice[i]
}

type marathonApps struct {
	Apps []marathonApp `json:"apps"`
}

type marathonApp struct {
	Id           string                `json:"id"`
	HealthChecks []marathonHealthCheck `json:"healthChecks"`
	Ports        []int                 `json:"ports"`
	Env          map[string]string     `json:"env"`
	Labels       map[string]string     `json:"labels"`
}

type marathonHealthCheck struct {
	Path      string `json:"path"`
	Protocol  string `json:"protocol"`
	PortIndex int    `json:"portIndex"`
}

func fetchMarathonApps(endpoint string, conf *configuration.Configuration) (map[string]marathonApp, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", endpoint+"/v2/apps", nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	if len(conf.Marathon.User) > 0 && len(conf.Marathon.Password) > 0 {
		req.SetBasicAuth(conf.Marathon.User, conf.Marathon.Password)
	}
	response, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	defer response.Body.Close()
	var appResponse marathonApps

	contents, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(contents, &appResponse)
	if err != nil {
		return nil, err
	}

	dataById := map[string]marathonApp{}

	for _, appConfig := range appResponse.Apps {
		if strings.TrimLeft(appConfig.Id, "/") == conf.Application.Id {
			dataById[appConfig.Id] = appConfig
		}
	}

	return dataById, nil
}

func fetchTasks(endpoint string, conf *configuration.Configuration) (map[string]marathonTaskList, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", endpoint+"/v2/tasks", nil)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	if len(conf.Marathon.User) > 0 && len(conf.Marathon.Password) > 0 {
		req.SetBasicAuth(conf.Marathon.User, conf.Marathon.Password)
	}
	response, err := client.Do(req)

	var tasks marathonTasks

	if err != nil {
		return nil, err
	}

	contents, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(contents, &tasks)
	if err != nil {
		return nil, err
	}

	taskList := tasks.Tasks
	sort.Sort(taskList)

	tasksById := map[string]marathonTaskList{}
	for _, task := range taskList {
		if tasksById[task.AppId] == nil {
			tasksById[task.AppId] = marathonTaskList{}
		}
		tasksById[task.AppId] = append(tasksById[task.AppId], task)
	}

	for _, task_list := range tasksById {
		sort.Sort(task_list)
	}

	return tasksById, nil
}

func createApps(tasksById map[string]marathonTaskList, marathonApps map[string]marathonApp) AppList {
	appMap := map[string]*App{}
	for _, mApp := range marathonApps {
		appPath := formPath(mApp)
		//log.Println("appPath", string(appPath))
		app, ok := appMap[appPath]
		if !ok {
			newApp := formApp(mApp, appPath)

			if endpointStr, ok := mApp.Env["HAPROXY_PORT"]; ok {
				endpoints := formEndpoints(endpointStr)
				newApp.Endpoints = endpoints
			}

			app = &newApp
			appMap[appPath] = app
		}

		tasks := formTasks(mApp, *app, tasksById)
		tasksJson, _ := json.Marshal(tasks)
		log.Println("tasks", string(tasksJson))
		app.Tasks = append(app.Tasks, tasks...)
		appJson, _ := json.Marshal(app)
		log.Println("app", string(appJson))
	}

	apps := AppList{}
	for _, app := range appMap {
		apps = append(apps, *app)
	}
	return apps
}

func formTasks(mApp marathonApp, app App, tasksById map[string]marathonTaskList) []Task {
	tasks := []Task{}
	for _, mTask := range tasksById[mApp.Id] {
		if len(mTask.Ports) > 0 {
			server := fmt.Sprintf("%s-%s", app.Frontend, mTask.Host)
			t := Task{
				Frontend: app.Frontend,
				Server:   server,
				Host:     mTask.Host,
				Port:     mTask.Ports[0],
				Ports:    mTask.Ports,
				Version:  mApp.Env["SRY_APP_VSN"],
				Weight:   1,
			}
			tasks = append(tasks, t)
		}
	}
	return tasks
}

func formEndpoints(str string) []Endpoint {
	epStrSlices := strings.Split(str, ",")
	endpoints := []Endpoint{}
	for _, epStr := range epStrSlices {
		var protocol string
		var bind int
		var err error
		epSlice := strings.Split(epStr, ":")
		if len(epSlice) != 2 && !strings.Contains(epStr, "tcp") && !strings.Contains(epStr, "http") {
			log.Println("[Warn] the HAPROXY_PORT env should be protocol with port, eg: tcp:8888, also the default protocol tcp is given if missing")
			protocol = "tcp"
			bind, err = strconv.Atoi(epSlice[0])
			if err != nil {
				log.Panicln("bad bind value", err.Error())
			}
		} else {
			protocol = epSlice[0]
			bind, err = strconv.Atoi(epSlice[1])
			if err != nil {
				log.Panicln("bad bind value", err.Error())
			}
		}
		endpoint := Endpoint{
			Protocol: protocol,
			Bind:     bind,
		}

		endpoints = append(endpoints, endpoint)
	}

	return endpoints
}

func formPath(mApp marathonApp) string {
	// Try to handle old app id format without slashes
	var appPath string
	if envAppId, ok := mApp.Env["SRY_APP_ID"]; ok {
		appPath = envAppId
	} else {
		appPath = mApp.Id
	}
	return strings.TrimPrefix(appPath, "/")
}

//formApp build App from marathonApp
func formApp(mApp marathonApp, appPath string) App {
	app := App{
		Id:              appPath,
		Frontend:        strings.Replace(appPath, "/", "::", -1),
		HealthCheckPath: parseHealthCheckPath(mApp.HealthChecks),
		Env:             mApp.Env,
		Labels:          mApp.Labels,
		CurVsn:          mApp.Env["SRY_APP_VSN"],
	}
	app.HealthChecks = make([]HealthCheck, 0, len(mApp.HealthChecks))
	for _, marathonCheck := range mApp.HealthChecks {
		check := HealthCheck{
			Protocol:  marathonCheck.Protocol,
			Path:      marathonCheck.Path,
			PortIndex: marathonCheck.PortIndex,
		}
		app.HealthChecks = append(app.HealthChecks, check)
	}

	if len(mApp.Ports) > 0 {
		app.ServicePort = mApp.Ports[0]
		app.ServicePorts = mApp.Ports
	}

	return app
}

func parseHealthCheckPath(checks []marathonHealthCheck) string {
	for _, check := range checks {
		if check.Protocol != "HTTP" {
			continue
		}
		return check.Path
	}
	return ""
}

/*
	Apps returns a struct that describes Marathon current app and their
	sub tasks information.

	Parameters:
		endpoint: Marathon HTTP endpoint, e.g. http://localhost:8080
*/
func FetchApps(maraconf configuration.Marathon, conf *configuration.Configuration) (AppList, error) {

	var applist AppList
	var err error

	// try all configured endpoints until one succeeds
	for _, url := range maraconf.Endpoints() {
		applist, err = _fetchApps(url, conf)
		if err == nil {
			return applist, err
		}
	}
	// return last error
	return nil, err
}

func _fetchApps(url string, conf *configuration.Configuration) (AppList, error) {
	tasks, err := fetchTasks(url, conf)
	if err != nil {
		return nil, err
	}

	marathonApps, err := fetchMarathonApps(url, conf)
	if err != nil {
		return nil, err
	}
	log.Println("got mapps", len(marathonApps))
	log.Println("got mtasks", len(tasks))
	apps := createApps(tasks, marathonApps)
	sort.Sort(apps)
	return apps, nil
}
