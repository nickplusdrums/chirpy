package main

import (
	"net/http"
)


func handlerReadiness(responseWriter http.ResponseWriter, request *http.Request) {
	responseWriter.Header().Set("Content-Type", "text/plain; charset=utf-8")
	responseWriter.WriteHeader(http.StatusOK)
	responseWriter.Write([]byte("OK"))
}




func main() {
	mux := http.NewServeMux()
	var server http.Server
	server.Addr = ":8080"
	server.Handler = mux
	mux.Handle("/app/", http.StripPrefix("/app", http.FileServer(http.Dir("."))))
	mux.HandleFunc("/healthz", handlerReadiness)



	err := server.ListenAndServe()
	if err != nil {
		return
	}
}



