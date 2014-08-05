package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"

	"code.google.com/p/go.net/publicsuffix"
)

const (
	jazzHubBaseUrl = "https://hub.jazz.net"
)

// A client for making http requests against a Jazz server with the provided credentials
// The client will execute the requests authenticating somewhat transparently when needed
type Client struct {
	httpClient      *http.Client
	userID          string
	encodedUserID   string
	encodedPassword string
	Log             *log.Logger
}

// Create a new client for making http requests against a Jazz server with the provided credentials
// The client will execute the requests authenticating somewhat transparently when needed
func NewClient(userID string, password string) (*Client, error) {
	jClient := &Client{}

	jClient.userID = userID
	jClient.encodedUserID = url.QueryEscape(userID)
	jClient.encodedPassword = url.QueryEscape(password)

	options := cookiejar.Options{
		PublicSuffixList: publicsuffix.List,
	}
	jar, err := cookiejar.New(&options)
	if err != nil {
		return nil, err
	}
	client := http.Client{Jar: jar}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client.Transport = tr
	client.CheckRedirect = nil

	jClient.httpClient = &client

	// Provide a no-op logger as the default
	jClient.Log = log.New(ioutil.Discard, "", log.LstdFlags)

	return jClient, nil
}

// Perform an http requests with this client
// Authentication is performed automatically
// In some instances both the response and error are nil in which case you must repeat your request
func (jClient *Client) Do(request *http.Request) (*http.Response, error) {
	jClient.Log.Println("Trying request:", request.URL)

	if jClient.userID == "" {
		// Set the user agent to firefox in order to get a guest token
		request.Header.Add("User-Agent", "Mozilla/5.0 (X11; Linux x86_64)")
	}

	resp, err := jClient.httpClient.Do(request)

	if err != nil {
		return nil, err
	}

	webAuthMsg := resp.Header.Get("x-com-ibm-team-repository-web-auth-msg")
	if webAuthMsg != "authrequired" && resp.StatusCode != 401 {
		// Request didn't require any further authentication. Return the result.
		return resp, nil
	}

	err = resp.Body.Close()

	if err != nil {
		return nil, err
	}

	// If credentials are provided then do the ccm OAuth dance to become authenticated
	if jClient.encodedPassword != "" {
		jClient.Log.Println("Authenticating using provided credentials for", jClient.userID)
		authReq, err := http.NewRequest("POST", jazzHubBaseUrl+"/ccm01/j_security_check",
			bytes.NewBufferString("j_username="+jClient.encodedUserID+"&j_password="+jClient.encodedPassword))

		if err != nil {
			return nil, err
		}

		authReq.Header = make(map[string][]string)
		authReq.Header["Content-Type"] = []string{"application/x-www-form-urlencoded"}

		resp, err = jClient.httpClient.Do(authReq)

		if err != nil {
			return nil, err
		}

		resp.Body.Close()

		if request.Body != nil {
			return nil, nil
		}
	} else {
		return nil, errors.New("Guest access was not granted")
	}

	jClient.Log.Println("Retrying request")
	resp, err = jClient.httpClient.Do(request)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

func (client *Client) findProject(name string) (Project, error) {
	projectEscaped := url.QueryEscape(name)

	// Discover the RTC repo for this project
	request, err := http.NewRequest("GET", jazzHubBaseUrl+"/manage/service/com.ibm.team.jazzhub.common.service.IProjectService/projectByName?projectName="+projectEscaped+"&refresh=true&includeMembers=false&includeHidden=true", nil)
	if err != nil {
		return Project{}, err
	}

	resp, err := client.Do(request)
	if err != nil {
		return Project{}, err
	}
	if resp.StatusCode != 200 {
		fmt.Printf("Response Status: %v\n", resp.StatusCode)
		b, _ := ioutil.ReadAll(resp.Body)
		fmt.Printf("Response Body\n%v\n", string(b))
		return Project{}, errors.New("Bad response from server")
	}
	result := &Project{}
	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return Project{}, err
	}
	err = json.Unmarshal(b, result)
	if err != nil {
		return Project{}, err
	}

	return *result, nil
}

func (client *Client) findCcmBaseUrl(projectName string) (string, error) {
	project, err := client.findProject(projectName)
	if err != nil {
		return "", err
	}

	return project.CcmBaseUrl, nil
}
