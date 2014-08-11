package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sync"

	"code.google.com/p/go.net/publicsuffix"
)

const (
	jazzHubBaseUrl = "https://hub.jazz.net"
)

// A client for making http requests against a Jazz server with the provided credentials
// The client will execute the requests authenticating somewhat transparently when needed
type Client struct {
	httpClient *http.Client
	userID     string
	password   string

	jazzIDmutex sync.Mutex
	jazzID2     string

	Log *log.Logger
}

// Create a new client for making http requests against a Jazz server with the provided credentials
// The client will execute the requests authenticating somewhat transparently when needed
func NewClient(userID string, password string) (*Client, error) {
	jClient := &Client{}

	jClient.userID = userID
	jClient.password = password

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

func (jClient *Client) GetJazzId() string {
	jClient.jazzIDmutex.Lock()
	defer jClient.jazzIDmutex.Unlock()

	return jClient.jazzID2
}

func (jClient *Client) SetJazzId(id string) {
	jClient.jazzIDmutex.Lock()
	defer jClient.jazzIDmutex.Unlock()

	jClient.jazzID2 = id
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

	// If credentials are provided then do the dance to become authenticated
	if jClient.password != "" {
		jClient.Log.Println("Authenticating using provided credentials for", jClient.userID)

		form := &url.Values{}
		form.Add("origin", "https://login.jazz.net")
		form.Add("username", jClient.userID)
		form.Add("password", jClient.password)

		authReq, err := http.NewRequest("POST", "https://login.jazz.net/sso/login.do", bytes.NewBufferString(form.Encode()))
		if err != nil {
			return nil, err
		}

		authReq.Header = make(map[string][]string)
		authReq.Header["Content-Type"] = []string{"application/x-www-form-urlencoded"}

		resp, err = jClient.httpClient.Do(authReq)
		if err != nil {
			return nil, err
		}

		authReq, err = http.NewRequest("GET", "https://login.jazz.net/psso/proxy/force?origin=https%3A%2F%2Flogin.jazz.net", nil)
		if err != nil {
			return nil, err
		}

		resp, err = jClient.httpClient.Do(authReq)
		if err != nil {
			return nil, err
		}

		// Unauthorized, authorize now
		if resp.StatusCode == 401 {
			type ForwardTo struct {
				RedirectUri string `json:"redirect_uri"`
				Client      string `json:"client_id"`
				State       string `json:"state"`
			}
			type Result1 struct {
				ForwardTo ForwardTo `json:"forwardTo"`
			}

			result1 := &Result1{}
			b, _ := ioutil.ReadAll(resp.Body)
			err := json.Unmarshal(b, result1)
			if err != nil {
				return nil, err
			}

			forwardTo := result1.ForwardTo
			client := forwardTo.Client
			state := forwardTo.State
			//redirectUri := forwardTo.RedirectUri

			authReq, err = http.NewRequest("GET", "https://login.jazz.net/sso/oauth/authorize?origin=https%3A%2F%2Flogin.jazz.net&response_type=code&client_id="+client+"&state="+state+"&redirect_uri=https%3A%2F%2Flogin.jazz.net%2Fpsso%2Fproxy%2Fauthorize", nil)
			if err != nil {
				return nil, err
			}

			resp, err = jClient.httpClient.Do(authReq)
			if err != nil {
				return nil, err
			}

			// The credentials did not work, abort with an error
			if resp.StatusCode != 200 {
				return nil, errorFromResponse(resp)
			}

			b, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			type Result2 struct {
				Code string `json:"code"`
			}
			result2 := &Result2{}
			err = json.Unmarshal(b, result2)
			if err != nil {
				return nil, err
			}

			code := result2.Code

			authReq, err = http.NewRequest("GET", "https://login.jazz.net/psso/proxy/authorize.do?origin=https%3A%2F%2Flogin.jazz.net&state="+state+"&code="+code, nil)
			if err != nil {
				return nil, err
			}

			resp, err = jClient.httpClient.Do(authReq)
			if err != nil {
				return nil, err
			}

			resp.Body.Close()

			// Last step is to discover the Jazz ID for the current user
			identReq, err := http.NewRequest("GET", "https://hub.jazz.net/manage/service/com.ibm.team.jazzhub.common.service.ICurrentUserService", nil)
			if err != nil {
				return nil, err
			}

			resp, err = jClient.httpClient.Do(identReq)
			if err != nil {
				return nil, err
			}

			if resp.StatusCode != 200 {
				return nil, errorFromResponse(resp)
			}

			b, _ = ioutil.ReadAll(resp.Body)
			resp.Body.Close()

			type IdentResult struct {
				UserId string `json:"userId"`
			}
			identResult := &IdentResult{}
			err = json.Unmarshal(b, identResult)
			if err != nil {
				return nil, err
			}

			jClient.SetJazzId(identResult.UserId)
		} else {
			panic(errorFromResponse(resp))
		}

		// If the initial request was a POST or PUT then send the special
		//  signal that the caller should repeat their request now that they
		//  are authenticated.
		if request.Body != nil {
			return nil, nil
		}
	} else {
		return nil, &JazzError{Msg: "Guest access was not granted"}
	}

	jClient.Log.Println("Retrying request")
	resp, err = jClient.httpClient.Do(request)

	if err != nil {
		return nil, err
	}

	return resp, nil
}

type Project struct {
	CcmBaseUrl string `json:"ccmBaseUrl"`
	ItemId     string `json:"itemId"`
	Name       string `json:"name"`
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
		return Project{}, errorFromResponse(resp)
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
