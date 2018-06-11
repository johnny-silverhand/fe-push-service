// Copyright (c) 2015 Mattermost, Inc. All Rights Reserved.
// See License.txt for license information.

package server

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/tylerb/graceful"
	"github.com/throttled/throttled"
	throttledStore "github.com/throttled/throttled/store"
)

const (
	HEADER_FORWARDED           = "X-Forwarded-For"
	HEADER_REAL_IP             = "X-Real-IP"
	WAIT_FOR_SERVER_SHUTDOWN   = time.Second * 5
	CONNECTION_TIMEOUT_SECONDS = 60
)

type NotificationServer interface {
	SendNotification(msg *PushNotification) PushResponse
	Initialize() bool
}

var servers map[string]NotificationServer = make(map[string]NotificationServer)

var gracefulServer *graceful.Server

func Start() {
	LogInfo("Push proxy server is initializing...")

	for _, settings := range CfgPP.ApplePushSettings {
		server := NewAppleNotificationServer(settings)
		if server.Initialize() {
			servers[settings.Type] = server
		}
	}

	for _, settings := range CfgPP.AndroidPushSettings {
		server := NewAndroideNotificationServer(settings)
		if server.Initialize() {
			servers[settings.Type] = server
		}
	}

	router := mux.NewRouter()
	var handler http.Handler = router
	vary := throttled.VaryBy{}
	vary.RemoteAddr = false
	vary.Headers = strings.Fields(CfgPP.ThrottleVaryByHeader)
	th := throttled.RateLimit(throttled.PerSec(CfgPP.ThrottlePerSec), &vary, throttledStore.NewMemStore(CfgPP.ThrottleMemoryStoreSize))

	th.DeniedHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		LogError(fmt.Sprintf("%v: code=429 ip=%v", r.URL.Path, GetIpAddress(r)))
		throttled.DefaultDeniedHandler.ServeHTTP(w, r)
	})

	handler = th.Throttle(router)

	router.HandleFunc("/", root).Methods("GET")

	metricCompatibleHandler := handleSendNotification
	if CfgPP.EnableMetrics {
		MetricsEnabled = true
		metrics := NewPrometheusHandler()
		router.Handle("/metrics", metrics).Methods("GET")
		metricCompatibleHandler = responseTimeMiddleware(handleSendNotification)
	}
	r := router.PathPrefix("/api/v4").Subrouter()
	r.HandleFunc("/send_push", metricCompatibleHandler).Methods("POST")

	go func() {
		gracefulServer = &graceful.Server{
			Timeout: WAIT_FOR_SERVER_SHUTDOWN,
			Server: &http.Server{
				Addr:         CfgPP.ListenAddress,
				Handler:      handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(handler),
				ReadTimeout:  time.Duration(CONNECTION_TIMEOUT_SECONDS) * time.Second,
				WriteTimeout: time.Duration(CONNECTION_TIMEOUT_SECONDS) * time.Second,
			},
		}
		err := gracefulServer.ListenAndServe()
		if err != nil {
			LogCritical(err.Error())
		}
	}()

	LogInfo("Server is listening on " + CfgPP.ListenAddress)
}

func Stop() {
	LogInfo("Stopping Server...")
	gracefulServer.Stop(WAIT_FOR_SERVER_SHUTDOWN)
}

func root(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("<html><body>Push Proxy</body></html>"))
}

func responseTimeMiddleware(f func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		f(w, r)
		observeServiceResponse(time.Since(start).Seconds())
	}
}

func handleSendNotification(w http.ResponseWriter, r *http.Request) {
	msg := PushNotificationFromJson(r.Body)

	if msg == nil {
		rMsg := LogError("Failed to read message body")
		w.Write([]byte(rMsg.ToJson()))
		incrementBadRequest()
		return
	}

	if len(msg.ServerId) == 0 {
		rMsg := LogError("Failed because of missing server Id")
		w.Write([]byte(rMsg.ToJson()))
		incrementBadRequest()
		return
	}

	if len(msg.DeviceId) == 0 {
		rMsg := LogError(fmt.Sprintf("Failed because of missing device Id serverId=%v", msg.ServerId))
		w.Write([]byte(rMsg.ToJson()))
		incrementBadRequest()
		return
	}

	if len(msg.Message) > 2047 {
		msg.Message = msg.Message[0:2046]
	}

	index := strings.Index(msg.DeviceId, ":")

	if index > -1 {
		msg.Platform = "android"
	}

	if server, ok := servers[msg.Platform]; ok {
		rMsg := server.SendNotification(msg)
		w.Write([]byte(rMsg.ToJson()))
		return
	} else {
		rMsg := LogError(fmt.Sprintf("Did not send message because of missing platform property type=%v serverId=%v", msg.Platform, msg.ServerId))
		w.Write([]byte(rMsg.ToJson()))
		incrementBadRequest()
		return
	}
}

func LogInfo(msg string) {
	Log("INFO", msg)
}

func LogError(msg string) PushResponse {
	Log("ERROR", msg)
	return NewErrorPushResponse(msg)
}

func LogCritical(msg string) {
	Log("CRIT", msg)
	panic(msg)
}

func Log(level string, msg string) {
	log.Printf("%v %v\n", level, msg)
}

func GetIpAddress(r *http.Request) string {
	address := r.Header.Get(HEADER_FORWARDED)

	if len(address) == 0 {
		address = r.Header.Get(HEADER_REAL_IP)
	}

	if len(address) == 0 {
		address, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	return address
}
