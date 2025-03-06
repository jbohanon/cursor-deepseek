package backend

import "net/http"

// TODO: Create a backend interface

type Backend interface {
	HandleModelsRequest(w http.ResponseWriter)
	HandleChatCompletions(w http.ResponseWriter, r *http.Request)
}
