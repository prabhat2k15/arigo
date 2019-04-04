package main

import (
	"fmt"
	"log"
	"net/http"
	"github.com/gorilla/mux"
	"github.com/arigo/controller"
)
func main() {
	fmt.Println("Sever running at port 8000")
	r := mux.NewRouter()
	r.HandleFunc("/cocapi/", controller.FetchCOC).Methods("POST")	
	log.Fatal(http.ListenAndServe(":8000", r))
}