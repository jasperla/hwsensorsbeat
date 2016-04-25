package main

import (
	"os"

	"github.com/elastic/beats/libbeat/beat"

	"github.com/jasperla/hwsensorsbeat/beater"
)

//import "github.com/davecgh/go-spew/spew"

func main() {
	if err := beat.Run("hwsensorsbeat", "", beater.New()); err != nil {
		os.Exit(1)
	}
}
