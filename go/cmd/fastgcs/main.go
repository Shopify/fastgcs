package main

import (
	"fmt"
	"io/ioutil"
	"log"

	fastgcs "github.com/Shopify/fastgcs/go"
)

func main() {
	fg, err := fastgcs.New()
	if err != nil {
		log.Fatal(err)
	}
	f, err := fg.Open("gs://shopify-dev/zodiac.constellations.json")
	if err != nil {
		log.Fatal(err)
	}
	data, err := ioutil.ReadAll(f)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(data))
}
