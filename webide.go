package main

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"net/http"
	"path"
	"strings"
	"time"
)

func addOrionHeaders(request *http.Request) {
	request.Header.Add("Jazz-Version", "2")
	request.Header.Add("Content-Type", "application/json; charset=UTF-8")
}

type OrionResponse struct {
	Result OrionResult
}

type OrionResult struct {
	HttpCode int
	JsonData interface{}
}

func waitForOrionResponse(client *Client, resp *http.Response, v interface{}) error {
	if resp.StatusCode == 200 {
		defer resp.Body.Close()
		if v != nil {
			b, _ := ioutil.ReadAll(resp.Body)
			err := json.Unmarshal(b, v)
			return err
		}
		return nil
	} else if resp.StatusCode != 202 {
		defer resp.Body.Close()
		return errorFromResponse(resp)
	}

	taskLocation := resp.Header.Get("Location")
	taskLocation = path.Join(jazzHubBaseUrl, taskLocation)
	taskLocation = strings.Replace(taskLocation, ":/", "://", 1)

	for {
		resp.Body.Close()
		<-time.After(100 * time.Millisecond)

		request, err := http.NewRequest("GET", taskLocation, nil)
		if err != nil {
			return err
		}
		request.Header.Add("Orion-Version", "1")

		resp, err := client.Do(request)
		if err != nil {
			return err
		}
		if resp.StatusCode != 200 {
			defer resp.Body.Close()
			return errorFromResponse(resp)
		}

		defer resp.Body.Close()

		b, err := ioutil.ReadAll(resp.Body)
		if err == io.EOF {
			// Spin and try again
			continue
		}
		
		orionResp := &OrionResponse{}
		orionResp.Result.JsonData = v
		err = json.Unmarshal(b, orionResp)
		if err != nil {
			return err
		}

		if orionResp.Result.HttpCode != 0 {
			return nil
		}
	}
}

type InitWebIdeProjectResult struct {
	Workspace InitWebIdeWorkspace `json:"workspace"`
}

type InitWebIdeWorkspace struct {
	ItemId string `json:"workspaceItemId"`
}

func initWebIdeProject(client *Client, project Project, userName string) (string, error) {
	url := path.Join(jazzHubBaseUrl, "/code/jazz/Project/")
	url = strings.Replace(url, ":/", "://", 1)

	request, err := http.NewRequest("POST", url, strings.NewReader(`{
		"Init": true,
		"repositoryUrl": "`+project.CcmBaseUrl+`",
		"projectName": "`+project.Name+`",
		"uuid": "`+project.ItemId+`",
		"user": "`+userName+`",
		"deleteSource": true,
		"initProject": true,
		"initReadme": false
	}`))
	if err != nil {
		return "", err
	}
	addOrionHeaders(request)

	resp, err := client.Do(request)
	if err != nil {
		return "", err
	}

	result := &InitWebIdeProjectResult{}
	err = waitForOrionResponse(client, resp, result)
	if err != nil {
		return "", err
	}

	return result.Workspace.ItemId, nil
}

func loadWorkspace(client *Client, projectName string, workspaceId string) error {
	if client.GetJazzId() == "" {
		return errors.New("Not logged in")
	}

	url := path.Join(jazzHubBaseUrl, "/code/jazz/Workspace/", workspaceId, "file", client.GetJazzId()+"-OrionContent", projectName)
	url = strings.Replace(url, ":/", "://", 1)

	request, err := http.NewRequest("POST", url, strings.NewReader(`{
		"Load": true
	}`))
	if err != nil {
		return err
	}
	addOrionHeaders(request)

	resp, err := client.Do(request)
	if err != nil {
		return err
	}

	var result struct{}
	err = waitForOrionResponse(client, resp, &result)
	if err != nil {
		return err
	}

	return nil
}
