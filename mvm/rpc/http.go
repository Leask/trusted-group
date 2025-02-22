package rpc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/MixinNetwork/trusted-group/mvm/store"
)

type RPC struct {
	store *store.BadgerStore
}

type Call struct {
	Id     string        `json:"id"`
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
}

func handlePanic(w http.ResponseWriter, r *http.Request) {
	rcv := recover()
	if rcv == nil {
		return
	}
	rdr := &Render{w: w}
	rdr.RenderError(fmt.Errorf("bad request"))
}

type Render struct {
	w  http.ResponseWriter
	id string
}

func (r *Render) RenderData(data interface{}) {
	body := map[string]interface{}{"data": data}
	r.render(body)
}

func (r *Render) RenderError(err error) {
	body := map[string]interface{}{"error": err.Error()}
	r.render(body)
}

func (r *Render) render(body map[string]interface{}) {
	if r.id != "" {
		body["id"] = r.id
	}
	b, err := json.Marshal(body)
	if err != nil {
		panic(err)
	}
	r.w.Header().Set("Content-Type", "application/json")
	r.w.WriteHeader(http.StatusOK)
	r.w.Write(b)
}

func (impl *RPC) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer handlePanic(w, r)

	rdr := &Render{w: w}
	if r.URL.Path != "/" || r.Method != "POST" {
		rdr.RenderError(fmt.Errorf("bad request %s %s", r.Method, r.URL.Path))
		return
	}

	var call Call
	d := json.NewDecoder(r.Body)
	d.UseNumber()
	if err := d.Decode(&call); err != nil {
		rdr.RenderError(fmt.Errorf("bad request %s", err.Error()))
		return
	}
	renderer := &Render{w: w, id: call.Id}
	switch call.Method {
	case "getinfo":
		info, err := getInfo(impl.store)
		if err != nil {
			renderer.RenderError(err)
		} else {
			renderer.RenderData(info)
		}
	default:
		renderer.RenderError(fmt.Errorf("invalid method %s", call.Method))
	}
}

func handleCORS(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type,Authorization,Mixin-Conversation-ID")
		w.Header().Set("Access-Control-Allow-Methods", "OPTIONS,GET,POST,DELETE")
		w.Header().Set("Access-Control-Max-Age", "600")
		if r.Method == "OPTIONS" {
			rdr := Render{w: w}
			rdr.render(map[string]interface{}{})
		} else {
			handler.ServeHTTP(w, r)
		}
	})
}

func NewServer(store *store.BadgerStore, port int) *http.Server {
	rpc := &RPC{store: store}
	handler := handleCORS(rpc)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%d", port),
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
	return server
}
