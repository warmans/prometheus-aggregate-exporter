package main

import (
	"log"
	"net/http"
)

func main() {
	fs := http.FileServer(http.Dir(""))
	http.Handle("/", fs)

	log.Println("Listening...")
	if err := http.ListenAndServe(":3000", nil); err != nil {
		panic(err)
	}
}
