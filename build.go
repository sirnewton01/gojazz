package main

import (
	"encoding/xml"
	"flag"
	"fmt"
	"hash/adler32"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	getBuildDefinitionTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>getBuildDefinition</method>
			<interface>com.ibm.team.build.internal.common.ITeamBuildService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:PrimitiveDataArg">
				<type>STRING</type>
				<value>%s</value>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`

	getBuildEngineTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
	xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
	xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>getBuildEngine</method>
			<interface>com.ibm.team.build.internal.common.ITeamBuildService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:PrimitiveDataArg">
				<type>STRING</type>
				<value>%s</value>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`

	startBuildTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
	xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
	xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
	xmlns:build="com.ibm.team.build"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>requestAndStartBuild</method>
			<interface>com.ibm.team.build.internal.common.ITeamBuildRequestService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexDataArg">
				<type>COMPLEX</type>
				<value xsi:type="build:BuildDefinitionHandle" itemId="%s">
					<stateId>%s</stateId>
				</value>
			</parameters>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexDataArg">
				<type>COMPLEX</type>
				<value xsi:type="build:BuildEngineHandle" itemId="%s">
					<stateId>%s</stateId>
				</value>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`

	fetchFullBuildResultTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
	xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
	xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
	xmlns:build="com.ibm.team.build"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>fetchOrRefreshItems</method>
			<interface>com.ibm.team.repository.common.internal.IRepositoryRemoteService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexArrayDataArg">
				<type>COMPLEX</type>
				<values xsi:type="build:BuildResultHandle" itemId="%s">
					<immutable>true</immutable>
				</values>
			</parameters>
			<parameters xsi:type="com.ibm.team.repository.common.services:PrimitiveArrayDataArg">
				<type>STRING</type>
				<values>tags</values>
				<values>contextId</values>
				<values>buildStatus</values>
				<values>ignoreWarnings</values>
				<values>stateId</values>
				<values>buildActivities</values>
				<values>label</values>
				<values>buildState</values>
				<values>itemId</values>
				<values>buildDefinition</values>
				<values>deleteAllowed</values>
				<values>buildStartTime</values>
				<values>buildTimeTaken</values>
				<values>personalBuild</values>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`

	saveBuildResultTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
	xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
	xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
	xmlns:build="com.ibm.team.build"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>save</method>
			<interface>com.ibm.team.build.internal.common.ITeamBuildBaseService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexDataArg">
				<type>COMPLEX</type>
				<value xsi:type="build:BuildResult" itemId="%s">
					<stateId xsi:nil="true"/>
					<immutable>%t</immutable>
					<contextId>%s</contextId>
					<modified xsi:nil="true"/>
					<workingCopy>true</workingCopy>
					<mergePredecessor xsi:nil="true"/>
					<workingCopyPredecessor>%s</workingCopyPredecessor>
					<workingCopyMergePredecessor xsi:nil="true"/>
					<predecessor xsi:nil="true"/>
					<buildStatus>%s</buildStatus>
					<buildState>%s</buildState>
					<label>%s</label>
					<buildTimeTaken>%d</buildTimeTaken>
					<buildStartTime>%d</buildStartTime>
					<ignoreWarnings>%t</ignoreWarnings>
					<tags>%s</tags>
					<deleteAllowed>%t</deleteAllowed>
					<personalBuild>%t</personalBuild>
					<modifiedBy xsi:nil="true"/>
					<buildDefinition  itemId="%s"  stateId="%s" />
					<buildActivities  itemId="%s" />
				</value>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`
	completeBuildTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
	xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
	xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
	xmlns:build="com.ibm.team.build"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>makeBuildComplete</method>
			<interface>com.ibm.team.build.internal.common.ITeamBuildRequestService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexDataArg">
				<type>COMPLEX</type>
				<value xsi:type="build:BuildResultHandle" itemId="%s">
				</value>
			</parameters>
			<parameters xsi:type="com.ibm.team.repository.common.services:PrimitiveDataArg">
				<type>BOOLEAN</type>
				<value>false</value>
			</parameters>
			<parameters xsi:type="com.ibm.team.repository.common.services:PrimitiveArrayDataArg">
				<type>STRING</type>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`

	publishLogTemplate = `<?xml version="1.0" encoding="UTF-8" ?>
<soapenv:Envelope
	xmlns:com.ibm.team.repository.common.services="http:///com/ibm/team/core/services.ecore"
	xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"
	xmlns:repository="com.ibm.team.repository"
	xmlns:build="com.ibm.team.build"
	xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
	<soapenv:Body>
		<request>
			<method>addBuildResultContributions</method>
			<interface>com.ibm.team.build.internal.common.ITeamBuildService</interface>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexDataArg">
				<type>COMPLEX</type>
				<value xsi:type="build:BuildResultHandle" itemId="%s">
					<stateId xsi:nil="true"/>
				</value>
			</parameters>
			<parameters xsi:type="com.ibm.team.repository.common.services:ComplexArrayDataArg">
				<type>COMPLEX</type>
				<values xsi:type="build:BuildResultContribution">
					<label>output.log</label>
					<contributionStatus>OK</contributionStatus>
					<impactsPrimaryResult>true</impactsPrimaryResult>
					<extendedContributionTypeId>com.ibm.team.build.common.model.IBuildResultContribution.log</extendedContributionTypeId>
					<extendedContributionData>
						<deltaPredecessor xsi:nil="true"/>
						<contentId>%s</contentId>
						<contentLength>%v</contentLength>
						<characterEncoding xsi:nil="true"/>
						<contentType>text/plain</contentType>
						<checksum>%v</checksum>
						<lineDelimiterSetting>0</lineDelimiterSetting>
						<lineDelimiterCount>0</lineDelimiterCount>
					</extendedContributionData>
					<extendedContributionProperties>
						<name>com.ibm.team.build.common.model.IBuildResultContribution.fileName</name>
						<value>%s</value>
					</extendedContributionProperties>
				</values>
			</parameters>
			<parameters xsi:type="com.ibm.team.repository.common.services:PrimitiveArrayDataArg">
				<type>STRING</type>
			</parameters>
		</request>
	</soapenv:Body>
</soapenv:Envelope>
`
)

type ItemHandleEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Soap    ItemHandleBody
}
type ItemHandleBody struct {
	XMLName  xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	Response ItemHandleResponse
}
type ItemHandleResponse struct {
	XMLName     xml.Name `xml:"response"`
	ReturnValue ItemHandleReturnValue
}
type ItemHandleReturnValue struct {
	XMLName xml.Name `xml:"returnValue"`
	Value   *ItemHandle
}
type ItemHandle struct {
	XMLName xml.Name `xml:"value"`
	ItemId  string   `xml:"itemId,attr"`
	StateId string   `xml:"stateId"`
}

func getBuildDefinition(client *Client, ccmBaseUrl string, id string) (ItemHandle, error) {
	buildDefHandle := ItemHandle{}

	buildServiceUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.build.internal.common.ITeamBuildService")
	buildServiceUrl = strings.Replace(buildServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("POST", buildServiceUrl, strings.NewReader(fmt.Sprintf(getBuildDefinitionTemplate, id)))
	if err != nil {
		return buildDefHandle, err
	}

	response, err := client.Do(request)
	if err != nil {
		return buildDefHandle, err
	}

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return buildDefHandle, err
	}

	result := &ItemHandleEnvelope{}
	result.Soap.Response.ReturnValue.Value = &buildDefHandle

	err = xml.Unmarshal(b, result)
	if err != nil {
		return buildDefHandle, err
	}

	return buildDefHandle, nil
}

func getBuildEngine(client *Client, ccmBaseUrl string, id string) (ItemHandle, error) {
	buildEngineHandle := ItemHandle{}

	buildServiceUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.build.internal.common.ITeamBuildService")
	buildServiceUrl = strings.Replace(buildServiceUrl, ":/", "://", 1)

	request, err := http.NewRequest("POST", buildServiceUrl, strings.NewReader(fmt.Sprintf(getBuildEngineTemplate, id)))
	if err != nil {
		return buildEngineHandle, err
	}

	response, err := client.Do(request)
	if err != nil {
		return buildEngineHandle, err
	}

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return buildEngineHandle, err
	}

	result := &ItemHandleEnvelope{}

	result.Soap.Response.ReturnValue.Value = &buildEngineHandle

	err = xml.Unmarshal(b, result)
	if err != nil {
		return buildEngineHandle, err
	}

	return buildEngineHandle, nil
}

type RequestBuildEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Soap    RequestBuildBody
}
type RequestBuildBody struct {
	XMLName  xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	Response RequestBuildResponse
}
type RequestBuildResponse struct {
	XMLName     xml.Name `xml:"response"`
	ReturnValue RequestBuildReturnValue
}
type RequestBuildReturnValue struct {
	XMLName xml.Name `xml:"returnValue"`
	Value   RequestBuildValue
}
type RequestBuildValue struct {
	XMLName      xml.Name `xml:"value"`
	BuildRequest *RequestBuildHandle
}
type RequestBuildHandle struct {
	XMLName           xml.Name `xml:"internalClientItems"`
	ItemId            string   `xml:"itemId,attr"`
	StateId           string   `xml:"stateId"`
	BuildResultHandle RequestBuildResultHandle
}
type RequestBuildResultHandle struct {
	XMLName xml.Name `xml:"buildResult"`
	ItemId  string   `xml:"itemId,attr"`
}

func startBuild(client *Client, ccmBaseUrl string, buildDefHandle ItemHandle, buildEngineHandle ItemHandle) (RequestBuildResultHandle, error) {
	requestBuildHandle := RequestBuildHandle{}

	requestBuildServiceUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.build.internal.common.ITeamBuildRequestService")
	requestBuildServiceUrl = strings.Replace(requestBuildServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("POST", requestBuildServiceUrl, strings.NewReader(fmt.Sprintf(startBuildTemplate, buildDefHandle.ItemId, buildDefHandle.StateId, buildEngineHandle.ItemId, buildEngineHandle.StateId)))
	if err != nil {
		return requestBuildHandle.BuildResultHandle, err
	}

	response, err := client.Do(request)
	if err != nil {
		return requestBuildHandle.BuildResultHandle, err
	}

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return requestBuildHandle.BuildResultHandle, err
	}

	requestBuildResult := &RequestBuildEnvelope{}

	requestBuildResult.Soap.Response.ReturnValue.Value.BuildRequest = &requestBuildHandle
	err = xml.Unmarshal(b, requestBuildResult)
	if err != nil {
		return requestBuildHandle.BuildResultHandle, err
	}

	return requestBuildHandle.BuildResultHandle, nil
}

type FullBuildResultEnvelope struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	Soap    FullBuildResultBody
}
type FullBuildResultBody struct {
	XMLName  xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	Response FullBuildResultResponse
}
type FullBuildResultResponse struct {
	XMLName     xml.Name `xml:"response"`
	ReturnValue FullBuildResultReturnValue
}
type FullBuildResultReturnValue struct {
	XMLName xml.Name `xml:"returnValue"`
	Value   FullBuildResultValue
}
type FullBuildResultValue struct {
	XMLName         xml.Name `xml:"value"`
	FullBuildResult *BuildResult
}
type BuildResult struct {
	XMLName         xml.Name `xml:"retrievedItems"`
	ItemId          string   `xml:"itemId,attr"`
	StateId         string   `xml:"stateId"`
	Immutable       bool     `xml:"immutable"`
	ContextId       string   `xml:"contextId"`
	BuildStatus     string   `xml:"buildStatus"`
	BuildState      string   `xml:"buildState"`
	Label           string   `xml:"label"`
	BuildTimeTaken  int64    `xml:"buildTimeTaken"`
	BuildStartTime  int64    `xml:"buildStartTime"`
	IgnoreWarnings  bool     `xml:"ignoreWarnings"`
	Tags            string   `xml:"tags"`
	DeleteAllowed   bool     `xml:"deleteAllowed"`
	PersonalBuild   bool     `xml:"personalBuild"`
	BuildDefinition BuildDefinitionResultHandle
	BuildActivities []BuildActivityResultHandle `xml:"buildActivities"`
}
type BuildDefinitionResultHandle struct {
	XMLName xml.Name `xml:"buildDefinition"`
	ItemId  string   `xml:"itemId,attr"`
	StateId string   `xml:"stateId,attr"`
}
type BuildActivityResultHandle struct {
	XMLName xml.Name `xml:"buildActivities"`
	ItemId  string   `xml:"itemId,attr"`
}

func fetchFullBuildResult(client *Client, ccmBaseUrl string, buildResultHandle RequestBuildResultHandle) (BuildResult, error) {
	buildResult := BuildResult{}

	requestBuildServiceUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.repository.common.internal.IRepositoryRemoteService")
	requestBuildServiceUrl = strings.Replace(requestBuildServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("POST", requestBuildServiceUrl, strings.NewReader(fmt.Sprintf(fetchFullBuildResultTemplate, buildResultHandle.ItemId)))
	if err != nil {
		return buildResult, err
	}

	response, err := client.Do(request)
	if err != nil {
		return buildResult, err
	}

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return buildResult, err
	}

	fullBuildResult := &FullBuildResultEnvelope{}
	fullBuildResult.Soap.Response.ReturnValue.Value.FullBuildResult = &buildResult

	err = xml.Unmarshal(b, fullBuildResult)
	if err != nil {
		return buildResult, err
	}

	return buildResult, nil
}

func saveFullBuildResult(client *Client, ccmBaseUrl string, buildResult BuildResult) error {
	requestBuildServiceUrl := path.Join(ccmBaseUrl, "/team/service/com.ibm.team.build.internal.common.ITeamBuildService")

	requestBody := fmt.Sprintf(saveBuildResultTemplate, buildResult.ItemId, buildResult.Immutable, buildResult.ContextId, buildResult.StateId, buildResult.BuildStatus, buildResult.BuildState, buildResult.Label, buildResult.BuildTimeTaken, buildResult.BuildStartTime, buildResult.IgnoreWarnings, buildResult.Tags, buildResult.DeleteAllowed, buildResult.PersonalBuild, buildResult.BuildDefinition.ItemId, buildResult.BuildDefinition.StateId, buildResult.BuildActivities[0].ItemId)

	reader := strings.NewReader(requestBody)

	requestBuildServiceUrl = strings.Replace(requestBuildServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("POST", requestBuildServiceUrl, reader)
	if err != nil {
		return err
	}

	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}

func completeBuild(client *Client, ccmBaseUrl string, buildResultHandle RequestBuildResultHandle) error {
	requestBuildServiceUrl := path.Join(ccmBaseUrl, "/service/com.ibm.team.build.internal.common.ITeamBuildRequestService")
	requestBuildServiceUrl = strings.Replace(requestBuildServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("POST", requestBuildServiceUrl, strings.NewReader(fmt.Sprintf(completeBuildTemplate, buildResultHandle.ItemId)))
	if err != nil {
		return err
	}

	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}

func publishLog(client *Client, ccmBaseUrl string, buildResultHandle RequestBuildResultHandle, fileName string, contentId string, contentLength int64, contentHash int64) error {
	requestBuildServiceUrl := path.Join(ccmBaseUrl, "/team/service/com.ibm.team.build.internal.common.ITeamBuildService")
	requestBuildServiceUrl = strings.Replace(requestBuildServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("POST", requestBuildServiceUrl, strings.NewReader(fmt.Sprintf(publishLogTemplate, buildResultHandle.ItemId, contentId, contentLength, contentHash, fileName)))
	if err != nil {
		return err
	}

	_, err = client.Do(request)
	if err != nil {
		return err
	}

	return nil
}

func uploadFile(client *Client, ccmBaseUrl string, filepath string) (string, int64, int64, error) {
	uuid := generateUUID()
	file, err := os.Open(filepath)
	if err != nil {
		return "", -1, -1, err
	}
	hash := adler32.New()
	_, err = io.Copy(hash, file)

	if err != nil {
		return "", -1, -1, err
	}
	file.Close()
	sum := hash.Sum(nil)
	sumInt := int64(sum[0])<<(8*3) | int64(sum[1])<<(8*2) | int64(sum[2])<<(8*1) | int64(sum[3])

	file, err = os.Open(filepath)

	uploadFileServiceUrl := path.Join(ccmBaseUrl, "/team/service/com.ibm.team.repository.common.transport.IDirectWritingContentService", uuid, strconv.FormatInt(sumInt, 10))
	uploadFileServiceUrl = strings.Replace(uploadFileServiceUrl, ":/", "://", 1)
	request, err := http.NewRequest("PUT", uploadFileServiceUrl, file)
	if err != nil {
		return "", -1, -1, err
	}
	request.Header.Add("Content-Type", "text/plain")

	s, err := os.Stat(file.Name())
	if err != nil {
		return "", -1, -1, err
	}

	request.ContentLength = s.Size()

	_, err = client.Do(request)
	file.Close()
	if err != nil {
		return "", -1, -1, err
	}

	return uuid, s.Size(), sumInt, nil
}

func buildDefaults() {
	fmt.Printf("gojazz build [options] -run build command\n")
	flag.PrintDefaults()
}

func buildOp() {
	commandIndex := -1
	for idx, arg := range os.Args {
		if arg == "-run" {
			commandIndex = idx
		}
	}

	if commandIndex == -1 {
		buildDefaults()
		return
	}

	buildCommands := os.Args[commandIndex+1:]
	os.Args = os.Args[:commandIndex]

	sandboxPath := flag.String("sandbox", "", "Location of the sandbox to sync the files")
	flag.Usage = buildDefaults
	flag.Parse()

	if *sandboxPath == "" {
		path, err := os.Getwd()
		if err != nil {
			panic(err)
		}

		path = findSandbox(path)
		sandboxPath = &path
	}
	
	// TODO explain to the user what's going on and the progress

	status, _ := scmStatus(*sandboxPath, NO_COPY)
	if status == nil {
		// No sandbox here, fail
		panic(simpleWarning("Sorry, there is no source code here to build. Run 'gojazz load' first to load the project's stream."))
	}

	projectName := status.metaData.projectName

	userId, password, err := getCredentials()
	if err != nil {
		panic(err)
	}

	client, err := NewClient(userId, password)
	if err != nil {
		panic(err)
	}

	ccmBaseUrl, err := client.findCcmBaseUrl(projectName)
	if err != nil {
		panic(err)
	}

	scmLoad(client, ccmBaseUrl, projectName, status.metaData.workspaceId, status.metaData.isstream, userId, *sandboxPath, status, true)

	// Find the build engine and build definition for the project
	// TODO Create them if necessary
	buildDefHandle, err := getBuildDefinition(client, ccmBaseUrl, projectName+" Default build")
	if err != nil {
		panic(err)
	}

	buildEngineHandle, err := getBuildEngine(client, ccmBaseUrl, projectName+" Default engine")
	if err != nil {
		panic(err)
	}

	// Start the build
	buildResultHandle, err := startBuild(client, ccmBaseUrl, buildDefHandle, buildEngineHandle)
	if err != nil {
		panic(err)
	}

	// FIXME this url doesn't seem to be working
	buildUrl := ccmBaseUrl + "/web/projects/" + projectName + "#action=com.ibm.team.build.viewResult&id=" + buildResultHandle.ItemId
	fmt.Printf("https://login.jazz.net/psso/proxy/jazzlogin?redirect_uri=%v\n", url.QueryEscape(buildUrl))

	// Update the build result with the build label and whether this is a personal build
	buildResult, err := fetchFullBuildResult(client, ccmBaseUrl, buildResultHandle)
	if err != nil {
		panic(err)
	}

	buildResult.Label = time.Now().Format("20060102-1504")
	buildResult.PersonalBuild = !status.metaData.isstream

	err = saveFullBuildResult(client, ccmBaseUrl, buildResult)
	if err != nil {
		panic(err)
	}

	// Launch the build process now and record the output
	cmd := exec.Command(buildCommands[0], buildCommands[1:]...)
	if err != nil {
		panic(err)
	}
	outputFile, err := ioutil.TempFile(os.TempDir(), "gojazz-build-output")
	if err != nil {
		panic(err)
	}
	errorFile, err := ioutil.TempFile(os.TempDir(), "gojazz-build-error")
	if err != nil {
		panic(err)
	}
	cmd.Stdout = outputFile
	cmd.Stderr = errorFile
	err = cmd.Run()
	outputFile.Close()
	errorFile.Close()

	isError := !cmd.ProcessState.Success()

	// Upload the output log
	contentId, contentLength, contentHash, err := uploadFile(client, ccmBaseUrl, outputFile.Name())
	if err != nil {
		panic(err)
	}
	err = publishLog(client, ccmBaseUrl, buildResultHandle, "stdout.txt", contentId, contentLength, contentHash)
	if err != nil {
		panic(err)
	}

	contentId, contentLength, contentHash, err = uploadFile(client, ccmBaseUrl, errorFile.Name())
	if err != nil {
		panic(err)
	}
	err = publishLog(client, ccmBaseUrl, buildResultHandle, "stderr.txt", contentId, contentLength, contentHash)
	if err != nil {
		panic(err)
	}

	if isError {
		// Update the build result with the the final status
		buildResult, err = fetchFullBuildResult(client, ccmBaseUrl, buildResultHandle)
		if err != nil {
			panic(err)
		}

		buildResult.BuildStatus = "ERROR"

		err = saveFullBuildResult(client, ccmBaseUrl, buildResult)
		if err != nil {
			panic(err)
		}
	}

	err = completeBuild(client, ccmBaseUrl, buildResultHandle)
	if err != nil {
		panic(err)
	}
}
