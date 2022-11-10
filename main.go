// Erase your computer if you call this API.
//
// Based on the Tailscale tshello.
package main

import (
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
	"strings"
	"time"

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

	tsclient, err := s.LocalClient()
	if err != nil {
		log.Fatal(err)
	}

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

	log.Fatal(http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimLeft(r.URL.Path, "/")
		if !(strings.HasPrefix(path, "erase/") || path == "erase-self") {
			http.Error(w, "Not found. Try /erase-self or /erase/<node-name>", 404)
			return
		}

		who, err := tsclient.WhoIs(r.Context(), r.RemoteAddr)
		if err != nil {
			log.Printf("Could not identify client: %v", err)
			http.Error(w, "Unauthorized", 401)
			return
		}

		if r.Method != "POST" {
			http.Error(w, "Method not allowed, only POSTs can erase", 405)
			return
		}

		var name string
		isSelf := path == "erase-self"
		if isSelf {
			name = firstLabel(who.Node.ComputedName)
		} else {
			name = strings.TrimPrefix(path, "erase/")
		}

		devices, err := enumerateMachines()
		if err != nil {
			log.Fatal(err)
		}

		device, err := getDeviceFromName(devices, name)
		if err != nil {
			log.Fatal(err)
		}
		if device == nil {
			device, err = getDeviceFromName(devices, strings.TrimSuffix(name, "-1"))
			if err != nil {
				log.Fatal(err)
			}
		}

		if device == nil {
			if isSelf {
				fmt.Fprintf(w, "I don't know who you are, %s!\n",
					html.EscapeString(firstLabel(who.Node.ComputedName)),
				)
			} else {
				fmt.Fprintf(w, "I don't know who %s is, %s!\n",
					html.EscapeString(name),
					html.EscapeString(firstLabel(who.Node.ComputedName)),
				)
			}

			log.Printf("no known device by name %s", name)
		} else {
			if err = sendErase(*device); err != nil {
				log.Printf("Failed to erase %s:", name, err)
			}

			fmt.Fprintf(w, `
⠀⠀⠀⠀⠀⠀⢀⣁⣤⣶⣶⡒⠒⠲⠾⣭⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⣿⡀⣸⠟⠛⠃⠀⣀⣀⠈⣷⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣴⠀⠀⠀⠀⠀⠀⠀
⠀⠀⠀⡠⠂⢠⠏⠀⠉⠀⠀⠀⠰⣿⠟⠀⠙⢧⡀⠀⠀⠀⠀⠀⠀⢀⠀⠀⢀⢀⡀⣼⣧⡾⠃⠀⠀⠀⠀⠀
⢀⠔⠀⣠⠔⠁⠀⠀⠀⠀⠀⠀⠀⠰⢄⡠⣶⢾⣽⡆⠀⠀⠀⠀⠄⢡⡀⢰⣾⣿⡀⠈⠵⠟⠛⠀⠀⠀⠀⠀
⠀⣠⠊⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⡟⠋⠉⠀⠀⠀⠀⣰⢦⣼⡷⣼⡏⢯⢉⣡⠖⠋⣩⡇⠀⠀⠀⠀
⣰⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇⠀⠀⠀⢺⣿⡄⣿⡄⣿⢿⠈⢁⡴⠋⠀⢀⣴⣋⡀⠀⠀⠀⠀
⡇⠀⠀⢰⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣿⠒⢛⣡⣸⡏⢹⡟⠻⠏⢀⡴⠋⠀⣠⣖⠻⠿⠿⣤⡀⠀⠀⠀
⡇⠀⠀⠈⣇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡟⠀⠛⢡⡞⠻⠟⠁⢀⡴⠋⢀⣤⣞⣛⣻⡆⠀⠀⠉⢇⠀⠀⠀
⣇⠀⠀⠀⠈⢦⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⠇⠀⠠⠏⠀⠀⢀⠴⠋⣀⠴⣿⠛⠛⠁⠈⠁⠀⠀⠀⠈⢧⠀⡄
⠸⡄⠀⠀⠀⠀⡇⠀⠀⢰⠃⠀⠈⣇⠀⠸⣦⡀⠀⠀⢀⡔⠁⣠⠞⠁⠀⡇⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⡀⠀
⠀⠙⣄⠀⠀⠀⣿⠀⠀⢸⠒⠒⠒⠻⡀⠀⣷⠬⣉⡶⠋⣠⠞⠁⠀⠀⠀⡇⠀⠀⠀⡀⠀⢠⠀⠀⠀⠘⡇⠀
⠀⠀⠈⠑⠦⠤⣽⣄⠀⢸⠤⠤⠤⠤⢷⡀⠸⣷⠋⣠⢾⡁⠀⠀⠀⠀⠀⡇⢠⠇⠀⢹⠀⢸⠃⠀⠀⣸⠃⠀
⠀⠀⠀⠀⠀⠀⠀⢹⠀⢸⠀⠀⠀⠀⠀⢈⠦⣀⣙⣻⡞⠃⠀⠀⠀⢀⡼⢡⠧⠤⠤⢸⠀⣾⠤⠤⠚⠁⠀⠀
⠀⠀⠀⠀⠀⠀⠀⢸⡀⠸⡄⠀⠀⠀⠀⣧⠴⠃⠉⠉⠁⠀⠀⠰⣾⡭⠔⠁⠀⠀⠀⡜⠀⡇⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠳⢤⣼⡆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠂⠄⠀⠀⠀⠀⠀⢰⣥⣴⠃⠀⠀⠀⠀⠀⠀
⠀⠀⠀⠀⠀⠀⠀⠀⠐⠀⠤⠐⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀
`+"%s, you're getting bonked! See you soon!\n",
				html.EscapeString(firstLabel(who.Node.ComputedName)),
			)
		}
	})))
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
