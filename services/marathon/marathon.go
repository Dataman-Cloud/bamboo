package marathon

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/QubitProducts/bamboo/configuration"
)

type MarathonTask struct {
	Id          string
	Host        string
	Ports       []int
	IpAddresses []TaskIpAdress
	Version     string
}

type TaskIpAdress struct {
	IpAddress string `json:"ipAddress"`
	Protocol  string `json:"protocol"`
}

type App struct {
	App MarathonApp `json:"app"`
}

type MarathonApp struct {
	Id        string            `json:"id"`
	Ports     []int             `json:"ports"`
	Env       map[string]string `json:"env"`
	Labels    map[string]string `json:"labels"`
	IpAddress AppIpAddress      `json:"ipAddress"`
	Tasks     []MarathonTask    `json:"tasks"`
}

type AppIpAddress struct {
	Discovery Discovery `json:"discovery"`
}

type Discovery struct {
	Ports []Port `json:"ports"`
}

type Port struct {
	Number   int    `json:"number"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
}

func FetchMarathonApp(conf *configuration.Configuration) (*MarathonApp, error) {
	var response *http.Response
	var err error
	for _, endpoint := range conf.Marathon.Endpoints() {
		client := &http.Client{}
		req, _ := http.NewRequest("GET", endpoint+"/v2/apps/"+conf.Application.Id, nil)
		req.Header.Add("Accept", "application/json")
		req.Header.Add("Content-Type", "application/json")
		if len(conf.Marathon.User) > 0 && len(conf.Marathon.Password) > 0 {
			req.SetBasicAuth(conf.Marathon.User, conf.Marathon.Password)
		}
		response, err = client.Do(req)
		if err != nil {
			continue
		} else {
			break
		}
	}

	if err != nil {
		return nil, err
	}

	var app App
	contents, err := ioutil.ReadAll(response.Body)
	defer response.Body.Close()
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(contents, &app)
	if err != nil {
		return nil, err
	}

	return &app.App, nil
}
