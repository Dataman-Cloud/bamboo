package api

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/QubitProducts/bamboo/configuration"
	"github.com/QubitProducts/bamboo/services/application"
	"github.com/QubitProducts/bamboo/services/haproxy"
	"github.com/QubitProducts/bamboo/services/service"
)

type StateAPI struct {
	Config     *configuration.Configuration
	Storage    service.Storage
	AppStorage application.Storage
}

func (state *StateAPI) Get(w http.ResponseWriter, r *http.Request) {
	templateData, _ := haproxy.GetTemplateData(state.Config)
	payload, _ := json.Marshal(templateData)
	io.WriteString(w, string(payload))
}
