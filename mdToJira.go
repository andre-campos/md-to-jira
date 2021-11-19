package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/akamensky/argparse"
	"github.com/howeyc/gopass"
	"github.com/njones/particle"
)

type TimeTracking struct {
	OriginalEstimate  string
	RemainingEstimate string
}

type MarkdownMetadata struct {
	Issuetype     string
	Key           string
	EpicLinkField string
	EpicLinkLabel string
	Summary       string
	Timetracking  TimeTracking
	Labels        []string
	Attachments   []string
}

type ParsedMarkdownFile struct {
	Metadata MarkdownMetadata
	Body     string
}

func readFiles(f string) []string {
	var files []string
	err := filepath.Walk(f, func(path string, info os.FileInfo, err error) error {
		if filepath.Ext(path) == ".md" {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		panic(err)
	}
	return files
}

func parseMarkdownWithMetadata(path string) (ParsedMarkdownFile, error) {
	var result ParsedMarkdownFile
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return result, err
	}
	_content := strings.ReplaceAll(string(content), "\r", "")
	body, err := particle.YAMLEncoding.DecodeString(_content, &result.Metadata)
	if err != nil {
		return result, err
	}
	result.Body = string(body)
	return result, nil
}

func main() {

	parser := argparse.NewParser("mdToJira", "Tool to convert a set of markdown files to Jira tickets")
	u := parser.String("u", "user", &argparse.Options{Required: true, Help: "Username to create tickets with"})
	p := parser.String("p", "password", &argparse.Options{Required: false, Help: "Password for user"})
	j := parser.String("j", "jira-server", &argparse.Options{Required: true, Help: "Jira server to create tickets on"})
	d := parser.Flag("d", "dry-run", &argparse.Options{Default: false, Help: "Dry run - doesn't create any tickets"})
	f := parser.String("f", "folder", &argparse.Options{Required: true, Help: "Folder to read files from"})
	err := parser.Parse(os.Args)
	if err != nil {
		fmt.Println(parser.Usage(err))
		return
	}
	if len(*p) == 0 {
		fmt.Println("Please enter your password")
		_p, err := gopass.GetPasswd()
		if err != nil {
			panic("Error reading the password")
		}
		*p = string(_p)
	}
	fmt.Println(*u)
	fmt.Println(*p)
	fmt.Println(*j)
	fmt.Println(*d)
	files := readFiles(*f)
	for _, file := range files {
		m, err := parseMarkdownWithMetadata(file)
		if err != nil {
			fmt.Println(m.Body)
			fmt.Println(m.Metadata)
		} else {
			fmt.Println(err)
		}
	}
}
