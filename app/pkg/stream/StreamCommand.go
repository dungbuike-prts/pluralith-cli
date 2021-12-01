package stream

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"strings"
	"time"

	"pluralith/pkg/comdb"
	"pluralith/pkg/ux"
)

func StreamCommand(command string, args []string) error {
	// Instantiate spinners
	streamSpinner := ux.NewSpinner("Apply Running", "Apply Completed", "Apply Failed")
	// Adapting spinner to destroy command
	if command == "destroy" {
		streamSpinner = ux.NewSpinner("Destroy Running", "Destroy Completed", "Destroy Failed")
	}

	// Get working directory for update emission
	workingDir, workingErr := os.Getwd()
	if workingErr != nil {
		return workingErr
	}

	// Emit apply begin update to UI
	comdb.PushComDBEvent(comdb.Event{
		Receiver:  "UI",
		Timestamp: time.Now().Unix(),
		Command:   "apply",
		Type:      "begin",
		Address:   "",
		Instances: make([]interface{}, 0),
		Path:      workingDir,
		Received:  false,
	})

	streamSpinner.Start()
	// Constructing command to execute
	cmd := exec.Command("terraform", append([]string{"apply"}, args...)...)

	// Define sinks for std data
	var errorSink bytes.Buffer

	// Redirect command std data
	cmd.Stderr = &errorSink

	// Initiate standard output pipe
	outStream, outErr := cmd.StdoutPipe()
	if outErr != nil {
		streamSpinner.Fail()
		return outErr
	}

	// Run terraform command
	cmdErr := cmd.Start()
	if cmdErr != nil {
		streamSpinner.Fail()
		return cmdErr
	}

	// Scan for command line updates
	applyScanner := bufio.NewScanner(outStream)
	applyScanner.Split(bufio.ScanLines)

	// While command line scan is running
	for applyScanner.Scan() {
		// Get current line json string
		jsonString := applyScanner.Text()
		// Decode json string to get event type and resource address
		event, address, decodeErr := DecodeStateStream(jsonString)
		if decodeErr != nil {
			streamSpinner.Fail()
			return decodeErr
		}

		// If address is given -> Resource event
		if address != "" {

			var instances []interface{}
			var fetchAttrErr error

			// If event complete -> Fetch resource instances with attributes
			if event == "apply_complete" {
				fetchedState, fetchErr := FetchState(address)
				if fetchErr != nil {
					return fetchErr
				}

				instances, fetchAttrErr = FetchResourceInstances(address, fetchedState)
				if fetchAttrErr != nil {
					return fetchAttrErr
				}
			}

			// // Emit current event update to UI
			comdb.PushComDBEvent(comdb.Event{
				Receiver:  "UI",
				Timestamp: time.Now().Unix(),
				Command:   "apply",
				Type:      strings.Split(event, "_")[1],
				Address:   address,
				Instances: instances,
				Path:      workingDir,
				Received:  false,
			})
		}
	}

	// Emit apply start update to UI
	comdb.PushComDBEvent(comdb.Event{
		Receiver:  "UI",
		Timestamp: time.Now().Unix(),
		Command:   "apply",
		Type:      "end",
		Address:   "",
		Instances: make([]interface{}, 0),
		Path:      workingDir,
		Received:  false,
	})

	streamSpinner.Success()

	return nil
}