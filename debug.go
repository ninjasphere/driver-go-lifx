// +build !release

package main

import "github.com/bugsnag/bugsnag-go"

func init() {

	bugsnag.Configure(bugsnag.Configuration{
		APIKey:       "57be7f895461a18014b0b325daf4ea3e",
		ReleaseStage: "development",
	})
}
