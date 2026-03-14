package main

import "strings"

type multiStringFlag []string

func (f *multiStringFlag) String() string {
	return strings.Join(*f, ", ")
}

func (f *multiStringFlag) Set(v string) error {
	*f = append(*f, v)
	return nil
}
