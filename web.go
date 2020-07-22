package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
)

type rESTfulResponse struct {
	Status  int
	Message string
	Data    interface{}
}

func isLocal(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	ip := net.ParseIP(host)
	return ip.IsLoopback()
}

func listHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	keys := []string{}
	for key := range saIndexes {
		keys = append(keys, key)
	}

	response := rESTfulResponse{0, "", keys}
	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/javascript")
	w.Write(jsonBytes)
	return
}

func queryHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	vars := mux.Vars(r)
	indexKey := vars["key"]
	if _, existed := saIndexes[indexKey]; indexKey == "" || !existed {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	qs := r.URL.Query()
	if qs.Get("q") == "" {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	t0 := time.Now()
	var group sync.WaitGroup
	data := make(map[string]uint64)
	ch := make(chan []Item)

	// 并发查询
	for i := 0; i < len(qs["q"]); i++ {
		group.Add(1)
		go func(key, query string) {
			ch <- indexLookup(saIndexes[key], query)
			group.Done()
		}(indexKey, qs["q"][i])
	}

	// 所有子查询完成后关闭频道
	go func() {
		group.Wait()
		close(ch)
	}()

	// 合并数据
	for items := range ch {
		for _, item := range items {
			data[item.Query] = item.Value
		}
	}

	t1 := time.Now()
	response := rESTfulResponse{
		0,
		fmt.Sprintf("%v items in %v", len(data), t1.Sub(t0)),
		data}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/javascript")
	w.Write(jsonBytes)
	return
}

func loadHandler(w http.ResponseWriter, r *http.Request) {
	// 只允许本地加载，避免因网络调用而需要增加的安全性检查
	if !isLocal(r.RemoteAddr) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	indexPath := r.PostForm.Get("path")
	indexKey := r.PostForm.Get("key")
	key, err := loadIndex(indexPath, indexKey)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	response := rESTfulResponse{
		0,
		fmt.Sprintf("Index [%v] loaded", key), nil}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/javascript")
	w.Write(jsonBytes)
	return
}

func unloadHandler(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r.RemoteAddr) {
		http.Error(w, http.StatusText(http.StatusForbidden), http.StatusForbidden)
		return
	}

	vars := mux.Vars(r)
	indexKey := vars["key"]
	if _, existed := saIndexes[indexKey]; indexKey == "" || !existed {
		http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}

	delete(saIndexes, indexKey)

	response := rESTfulResponse{
		0,
		fmt.Sprintf("Index [%v] deleted", indexKey), nil}

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/javascript")
	w.Write(jsonBytes)
	return
}

func startWebServer(address string) error {
	router := mux.NewRouter()
	router.HandleFunc("/indexes/", listHandler).Methods("GET")
	router.HandleFunc("/index/{key:[0-9a-zA-Z]+}/", queryHandler).Methods("GET")
	router.HandleFunc("/index/", loadHandler).Methods("POST")
	router.HandleFunc("/index/{key:[0-9a-zA-Z]+}/", unloadHandler).Methods("DELETE")

	loggedRouter := handlers.CombinedLoggingHandler(os.Stdout, router)
	http.Handle("/", loggedRouter)
	return http.ListenAndServe(address, nil)
}
