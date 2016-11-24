package event_bus

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/smallnest/goreq"

	"github.com/QubitProducts/bamboo/services/marathon"
)

func handleF5Update(h *Handlers) {
	f5 := F5{h.Conf}
	// create pool
	err := f5.CreatePool(h)
	if err != nil {
		log.Println("create pool error: ", err.Error())
	}

	err = f5.CreateVirtualServer(h)
	if err != nil {
		log.Println("create virtual server error: ", err.Error())
	}
}

func (f5 *F5) CreateVirtualServer(h *Handlers) error {
	app, err := marathon.FetchMarathonApp(h.Conf.Marathon.Endpoint, h.AppId, h.Conf)
	if err != nil {
		return err
	}

	if app.Env["VIRTUAL_SERVER_DESTINATION"] == "" {
		return errors.New("Please set Env VIRTUAL_SERVER_DESTINATION")
	}

	virtualServer := VirtualServer{
		Name:        fmt.Sprintf("%s_%s", "pufa", h.AppId),
		Description: h.AppId,
		Destination: app.Env["VIRTUAL_SERVER_DESTINATION"],
		IpProtocol:  "TCP",
		Pool:        h.AppId,
	}

	virtualServerJson, err := json.Marshal(virtualServer)
	if err != nil {
		return err
	}

	request := f5.request()
	resp, _, errs := request.Post(h.Conf.F5.EndPoint + "/mgmt/tm/ltm/virtual").ContentType("json").SendMapString(string(virtualServerJson)).End()
	if len(errs) != 0 {
		return errs[0]
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("the response code is " + string(resp.StatusCode))
	}

	return nil
}

func (f5 *F5) CreatePool(h *Handlers) error {
	tasks, err := marathon.FetchTasks(h.Conf.Marathon, h.AppId, h.Conf)
	if err != nil {
		return err
	}

	var members []Member
	for _, task := range tasks {
		member := Member{Address: task.Host, Name: fmt.Sprintf("%s:%d", task.Host, task.Ports[0])}
		members = append(members, member)
	}

	pool := Pool{
		Name:              fmt.Sprintf("%s_%s", "pufa", h.AppId),
		LoadBalancingMode: "round-robin",
		Monitor:           "",
		Members:           members,
	}

	poolJson, err := json.Marshal(pool)
	if err != nil {
		return err
	}

	request := f5.request()
	resp, _, errs := request.Post(h.Conf.F5.EndPoint + "/mgmt/tm/ltm/pool").ContentType("json").SendMapString(string(poolJson)).End()
	if len(errs) != 0 {
		return errs[0]
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusConflict {
		return errors.New("the response code is " + string(resp.StatusCode))
	}

	if resp.StatusCode == http.StatusConflict {
		err = f5.UpdatePoolMember(h)
		if err != nil {
			return err
		}
	}

	return nil
}

func (f5 *F5) UpdatePoolMember(h *Handlers) error {
	tasks, err := marathon.FetchAppTasks(h.Conf.Marathon.Endpoint, h.AppId, h.Conf)
	if err != nil {
		return err
	}

	var members []Member
	for _, task := range tasks {
		member := Member{Address: task.Host, Name: fmt.Sprintf("%s:%d", task.Host, task.Ports[0])}
		members = append(members, member)
	}

	pool := Pool{
		Name:              fmt.Sprintf("%s_%s", "pufa", h.AppId),
		LoadBalancingMode: "round-robin",
		Monitor:           "",
		Members:           members,
	}

	poolJson, err := json.Marshal(pool)
	if err != nil {
		return err
	}

	request := f5.request()
	resp, _, errs := request.Post(h.Conf.F5.EndPoint + "/mgmt/tm/ltm/pool/" + pool.Name).ContentType("json").SendMapString(string(poolJson)).End()
	if resp.StatusCode != http.StatusOK {
		return errors.New("the response code is " + string(resp.StatusCode))
	}
	if len(errs) != 0 {
		return errs[0]
	}

	return nil
}

func (f5 *F5) request() *goreq.GoReq {
	return goreq.New().SetBasicAuth(f5.Config.F5.UserName, f5.Config.F5.Password).TLSClientConfig(&tls.Config{InsecureSkipVerify: true})
}
