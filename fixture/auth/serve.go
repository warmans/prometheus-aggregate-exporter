package main

import (
	"log"
	"net/http"
	"os"
)

const (
	authUser   = "prometheus_scraper"
	authPass   = "test-password-123"
	authBearer = "test-bearer-token-123"
)

func main() {
	fs := http.FileServer(http.Dir(".."))

	http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if ok && user == authUser && pass == authPass {
			fs.ServeHTTP(rw, r)
			return
		}
		if r.Header.Get("Authorization") == "Bearer "+authBearer {
			fs.ServeHTTP(rw, r)
			return
		}
		rw.Header().Set("WWW-Authenticate", `Basic realm="metrics"`)
		http.Error(rw, "Unauthorized", http.StatusUnauthorized)
	})

	port := os.Getenv("AUTH_FIXTURE_PORT")
	if port == "" {
		port = "3011"
	}
	addr := ":" + port

	log.Printf("Listening with Basic/Bearer Auth on %s ...", addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		panic(err)
	}
}
