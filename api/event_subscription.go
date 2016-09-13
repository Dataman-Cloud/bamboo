package api

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/QubitProducts/bamboo/configuration"
	eb "github.com/QubitProducts/bamboo/services/event_bus"
)

type EventSubscriptionAPI struct {
	Conf     *configuration.Configuration
	EventBus *eb.EventBus
}

func (sub *EventSubscriptionAPI) Callback(w http.ResponseWriter, r *http.Request) {
	payload, _ := ioutil.ReadAll(r.Body)

	sub.Notify(payload)

	io.WriteString(w, "Got it!")
}

func (sub *EventSubscriptionAPI) Notify(payload []byte) {

	var event eb.MarathonEvent
	err := json.Unmarshal(payload, &event)

	if err != nil {
		log.Printf("Unable to decode JSON Marathon Event request: %s \n", string(payload))
	}
	sub.NotifyByEvent(payload, event)
}

func (sub *EventSubscriptionAPI) NotifyByEvent(payload []byte, event eb.MarathonEvent) {
	var err error
	switch event.EventType {
	case "api_post_event":
		var apiPostEvent eb.ApiPostEvent
		err = json.Unmarshal(payload, &apiPostEvent)
		event.AppId = apiPostEvent.AppDefinition.Id

	case "status_update_event":
		var statusUpdateEvent eb.StatusUpdateEvent
		err = json.Unmarshal(payload, &statusUpdateEvent)
		event.AppId = statusUpdateEvent.AppId

	}

	if err != nil {
		log.Printf("Unable to decode JSON Marathon Event request: %s \n", string(payload))
	}

	sub.EventBus.Publish(event)
}
