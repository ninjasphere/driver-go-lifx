// +build release

package main

import "github.com/ninjasphere/go-ninja/bugs"

// BugsKey key used for reporting bugs
const BugsKey = "57be7f895461a18014b0b325daf4ea3e"

func init() {
	bugs.Configure("release", BugsKey)
}
