package main

import (
	"log"
	"net/http"
)

const (
	authUser = "prometheus_scraper"
	authPass = "test-password-123"
)

func main() {
	fs := http.FileServer(http.Dir(""))

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != authUser || pass != authPass {
			rw.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
			http.Error(rw, "Unauthorized", http.StatusUnauthorized)
			return
		}
		fs.ServeHTTP(rw, r)
	})

	log.Println("Listening with Basic Auth on :3001 ...")
	if err := http.ListenAndServe(":3001", nil); err != nil {
		panic(err)
	}
}
