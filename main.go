// Erase your computer if you call this API.
//
// Based on the Tailscale tshello.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"tailscale.com/client/tailscale"
	"tailscale.com/client/tailscale/apitype"
	"tailscale.com/tsnet"
)

var (
	addr = flag.String("addr", ":80", "address to listen on")
)

type DeviceListResponse struct {
	Response []DeviceList `json:response`
}

type DeviceList struct {
	Devices  []Device `json:devices`
	Rows     int      `json:rows`
	PageSize int      `json:page_size`
	Page     int      `json:page`
}

type Device struct {
	DeviceUDID    string `json:deviceudid`
	LocalHostName string `json:LocalHostname`
}

var tsclient *tailscale.LocalClient

func main() {
	flag.Parse()

	log.Println("Verifying we can fetch machines from Mosyle...")
	_, err := enumerateMachines()
	if err != nil {
		log.Fatal(err)
	}

	s := &tsnet.Server{
		AuthKey:   os.Getenv("TS_AUTHKEY"),
		Ephemeral: true,
		Hostname:  "bonk",
	}

	tsclient_, err := s.LocalClient()
	if err != nil {
		log.Fatal(err)
	}
	tsclient = tsclient_

	defer s.Close()
	ln, err := s.Listen("tcp", *addr)
	if err != nil {
		log.Println(err)
	}
	defer ln.Close()

	if *addr == ":443" {
		ln = tls.NewListener(ln, &tls.Config{
			GetCertificate: tsclient.GetCertificate,
		})
	}

	http.HandleFunc("/erase/", withEraseContext(erase))
	http.HandleFunc("/erase-self", withEraseContext(eraseSelf))
	http.HandleFunc("/erase-all", withEraseContext(eraseAll))
	http.HandleFunc("/", notFound)

	log.Fatal(http.Serve(ln, nil))
}

type ctxKey struct{}
type eraseContext struct {
	client  *apitype.WhoIsResponse
	devices []Device
}

func withEraseContext(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		client, err := tsclient.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			log.Printf("Could not identify client: %v", err)
			http.Error(w, "Unauthorized", 401)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed, only POSTs can erase", 405)
			return
		}

		devices, err := enumerateMachines()
		if err != nil {
			log.Fatal(err)
		}

		ctx := eraseContext{client, devices}
		fn(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, ctx)))
	}
}

func eraseSelf(w http.ResponseWriter, r *http.Request) {
	bonk(w, r, r.Context().Value(ctxKey{}).(eraseContext).client.Node.ComputedName)
}

var nameRegex = regexp.MustCompile("/erase/([^/]+)")

func erase(w http.ResponseWriter, r *http.Request) {
	name := nameRegex.FindStringSubmatch(r.URL.Path)[1]
	bonk(w, r, name)
}

// This sends the erase requests one-by-one synchronously. It would be
// nicer to submit them in parallel and return a job ID or something
// which can later be queried for progress, but at the current scale (4
// machines) I think waiting a few seconds per machine is still
// acceptable.
func eraseAll(w http.ResponseWriter, r *http.Request) {
	context := r.Context().Value(ctxKey{}).(eraseContext)
	anyFailed := false
	messages := make([]string, 0, len(context.devices))
	for _, device := range context.devices {
		if err := sendErase(device); err != nil {
			anyFailed = true
			messages = append(messages, fmt.Sprintf("could not bonk %s: %s\n", device.LocalHostName, err))
		} else {
			messages = append(messages, fmt.Sprintf("bonking %s!\n", device.LocalHostName))
		}
	}
	if anyFailed {
		w.WriteHeader(500)
	}
	for _, msg := range messages {
		w.Write([]byte("IT'S A BONK PARTY!"))
		w.Write([]byte(msg))
	}
}

func bonk(w http.ResponseWriter, r *http.Request, name string) {

	context := r.Context().Value(ctxKey{}).(eraseContext)

	device, err := getDeviceFromName(context.devices, name)
	if err != nil {
		log.Fatal(err)
	}
	if device == nil {
		// REVIEW: is this actually a thing that mosyle does?
		device, err = getDeviceFromName(context.devices, strings.TrimSuffix(name, "-1"))
		if err != nil {
			log.Fatal(err)
		}
	}

	if device == nil {
		fmt.Fprintf(w, "I don't know who %s is, %s!\n",
			html.EscapeString(name),
			html.EscapeString(firstLabel(context.client.Node.ComputedName)),
		)
		log.Printf("no known device by name %s", name)

	} else {
		//if err = sendErase(*device); err != nil {
		//	log.Printf("Failed to erase %s:", name, err)
		//}

		fmt.Fprintf(w, "%s is not getting bonked now, sorry! (Linus is hacking on prod)",
			html.EscapeString(name),
		)
	}
}

func notFound(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not found. Try /erase-self or /erase/<node-name>", 404)
	return
}

func firstLabel(s string) string {
	if i := strings.Index(s, "."); i != -1 {
		return s[:i]
	}
	return s
}

func getDeviceFromName(devices []Device, name string) (*Device, error) {
	matching_udids := []Device{}
	for _, dev := range devices {
		if dev.LocalHostName == name {
			matching_udids = append(matching_udids, dev)
		}
	}

	if len(matching_udids) == 0 {
		return nil, nil
	}

	if len(matching_udids) > 1 {
		return nil, errors.New("Multiple machines with matching names")
	}

	return &matching_udids[0], nil
}

func enumerateMachines() ([]Device, error) {
	data := url.Values{
		"operation":   {"list"},
		"options[os]": {"mac"},
	}

	req, err := http.NewRequest(http.MethodPost, "https://businessapi.mosyle.com/v1/devices", strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+os.Getenv("MOSYLE_AUTHORIZATION"))
	req.Header.Set("accesstoken", os.Getenv("MOSYLE_ACCESS_TOKEN"))

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	fmt.Printf("enumerating machines: %v\n", res)

	body, _ := ioutil.ReadAll(res.Body)

	obj := &DeviceListResponse{}
	if err := json.Unmarshal(body, &obj); err != nil {
		fmt.Println("error unmarshaling body:", string(body))
		return nil, err
	}

	if obj.Response == nil {
		return nil, fmt.Errorf("nil Response in Device list")
	}

	if len(obj.Response) != 1 {
		return nil, fmt.Errorf("Too many Responses in Device list")
	}

	if obj.Response[0].Devices == nil {
		return nil, fmt.Errorf("Response's Devices list is nil")
	}

	devices_response := obj.Response[0]
	if devices_response.PageSize == devices_response.Rows {
		fmt.Println("Number of devices returend matches the page size! Could be losing devices, since we don't paginate.")
	}

	if devices_response.Devices == nil {
		return nil, fmt.Errorf("Response's Devices list is nil")
	}

	return devices_response.Devices, nil
}

func sendErase(device Device) error {
	data := url.Values{
		"operation":                     {"wipe_devices"},
		"devices[]":                     {device.DeviceUDID},
		"options[pin_code]":             {"123456"},
		"options[ObliterationBehavior]": {"DoNotObliterate"},
	}

	req, err := http.NewRequest(http.MethodPost, "https://businessapi.mosyle.com/v1/devices", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+os.Getenv("MOSYLE_AUTHORIZATION"))
	req.Header.Set("accesstoken", os.Getenv("MOSYLE_ACCESS_TOKEN"))

	client := http.Client{
		Timeout: 5 * time.Second,
	}

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	log.Printf("sending wipe: %v\n", res)
	body, _ := ioutil.ReadAll(res.Body)
	log.Printf("reply: %v\n", body)

	return nil
}
