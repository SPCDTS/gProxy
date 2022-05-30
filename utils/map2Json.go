package utils

import (
	"encoding/json"
	"log"
	"os"
)

const dataFile = "../proxyEntry.json"

func Map2File(dic map[string]interface{}) (err error) {
	fPtr, err := os.Create(dataFile)

	if err != nil {
		return
	}

	log.Printf("正在写入: %s\n", dataFile)
	defer fPtr.Close()
	encoder := json.NewEncoder(fPtr)
	err = encoder.Encode(dic)
	return
}

func File2Map(dic interface{}) (err error) {
	fPtr, err := os.Open(dataFile)
	if err != nil {
		return
	}
	defer fPtr.Close()
	decoder := json.NewDecoder(fPtr)
	decoder.Decode(dic)
	return
}
