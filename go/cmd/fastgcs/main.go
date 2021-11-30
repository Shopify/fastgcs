package main

import (
	"log"

	fastgcs "github.com/Shopify/fastgcs/go"
)

func main() {
	fg, err := fastgcs.New()
	if err != nil {
		log.Fatal(err)
	}
	fg.Open("gs://neato/foobar")
}
