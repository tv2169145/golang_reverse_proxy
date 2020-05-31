package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	proxyHost = "127.0.0.1:2000"
	toHost1 = "http://127.0.0.1:2003"
	toHost2 = "http://127.0.0.1:2004"
)

func main() {
	url1, err1 := url.Parse(toHost1)
	if err1 != nil {
		log.Println(err1)
	}

	url2, err2 := url.Parse(toHost2)
	if err2 != nil {
		log.Println(err2)
	}
	targets := []*url.URL{url1, url2}
	proxy := NewCustomProxy(targets)
	log.Println("start proxy at:", proxyHost)
	log.Fatal(http.ListenAndServe(proxyHost, proxy))

}

var transport = &http.Transport{
	DialContext: (&net.Dialer{
		Timeout: time.Second * 30,
		KeepAlive: 30 * time.Second, //長連接超時時間
	}).DialContext,
	MaxIdleConns:          100,              //最大空閑連接
	IdleConnTimeout:       90 * time.Second, //空閑超時時間
	TLSHandshakeTimeout:   10 * time.Second, //tls握手超時時間
	ExpectContinueTimeout: 1 * time.Second,  //100-continue 超時時間
}

func NewCustomProxy(targets []*url.URL) *httputil.ReverseProxy {
	// 轉發規則
	director := func(req *http.Request) {
		// 隨機選擇轉發位置
		targetIndex := rand.Intn(len(targets))
		target := targets[targetIndex]
		fmt.Println("send to :", target.Host)
		targetQuery := target.RawQuery

		// 重組轉發url
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.Host = target.Host

		req.URL.Path = getNewPath(target.Path, req.URL.Path)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "user-agent")
		}
	}

	// 修改response
	modifyFunc := func(resp *http.Response) error {
		if strings.Contains(resp.Header.Get("Connection"), "Upgrade") {
			return nil
		}

		var payload []byte
		var readErr error
		if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
			gr, err := gzip.NewReader(resp.Body)
			if err != nil {
				return err
			}
			payload, readErr = ioutil.ReadAll(gr)
			resp.Header.Del("Content-Encoding")
		} else {
			payload, readErr = ioutil.ReadAll(resp.Body)
		}
		if readErr != nil {
			return readErr
		}

		if resp.StatusCode != http.StatusOK {
			payload = []byte("StatusCode error:" + string(payload))
		}

		// 因為前面先讀了body, 這裡需重寫
		resp.Body = ioutil.NopCloser(bytes.NewBuffer(payload))
		resp.ContentLength = int64(len(payload))
		resp.Header.Set("Content-Length", strconv.FormatInt(int64(len(payload)), 10))
		return nil
	}

	errorFunc := func(w http.ResponseWriter, r *http.Request, err error) {
		http.Error(w, "ErrorHandler error:" + err.Error(), 500)
	}


	return &httputil.ReverseProxy{
		Director: director,
		ModifyResponse: modifyFunc,
		ErrorHandler: errorFunc,
		Transport: transport,
	}
}

func getNewPath(targetPath, requestPath string) string {
	targetPathHasSlash := strings.HasSuffix(targetPath, "/")
	requestPathHasSlash := strings.HasPrefix(requestPath, "/")
	switch {
	case targetPathHasSlash && requestPathHasSlash :
		return targetPath + requestPath[1:]
	case !targetPathHasSlash && !requestPathHasSlash :
		return targetPath + "/" + requestPath
	}
	return targetPath + requestPath
}
