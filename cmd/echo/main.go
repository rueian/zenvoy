package main

import (
	"log"
	"net/http"
	"net/http/httputil"
)

func main() {
	log.Println("starting echo on :8080")
	http.ListenAndServe(":8080", http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		bs, err := httputil.DumpRequest(request, true)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
		}
		w.Write(bs)
	}))
}
