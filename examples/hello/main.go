package main

import (
	"fmt"
	"net/http"
	"os"
)

func main() {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hello, World!"))
	})
	http.ListenAndServe(fmt.Sprintf("%v:%v", os.Getenv("BIND_ADDR"), os.Getenv("BIND_PORT")), handler)
}
