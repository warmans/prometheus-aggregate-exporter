package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

var updateGoldenFile = flag.Bool("update.golden", false, "update golden files")

func TestMain(m *testing.M) {

	flag.Parse()
	os.Exit(m.Run())
}

func mustOpenFile(name string, flag int) *os.File {
	file, err := os.OpenFile(fmt.Sprintf("fixture/%s", name), flag, 0666)
	if err != nil {
		panic(err)
	}
	return file
}

func mustReadAll(r io.Reader) string {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		panic("failed to read all from file: " + err.Error())
	}
	return string(b[:])
}

//todo: re-implement tests
