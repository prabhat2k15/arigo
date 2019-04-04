package controller

import(
	"fmt"
	"net/http"
	"encoding/json"
	// "github.com/gorilla/mux"


)

type Caller struct{
	Callto string `json:"callto"`
	Callfrom string `json:"callfrom"`
}

func FetchCOC(res http.ResponseWriter, req *http.Request){
	fmt.Println("FetchCOC invoked")
	var caller Caller
	_ = json.NewDecoder(req.Body).Decode(&caller)
	fmt.Println(caller)
	fmt.Println("callto:", caller.Callto)
	fmt.Println("callfrom:", caller.Callfrom)
	res.Header().Add("Content-Type", "application/json")
	json.NewEncoder(res).Encode(caller)
}

