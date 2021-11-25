package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/akamensky/argparse"
	"github.com/howeyc/gopass"
	"github.com/interviewstreet/go-jira"
	cf "github.com/kentaro-m/blackfriday-confluence"
	"github.com/njones/particle"
	bf "github.com/russross/blackfriday/v2"
	"github.com/trivago/tgo/tcontainer"
)

type TimeTracking struct {
	OriginalEstimate  string `yaml:"originalEstimate"`
	RemainingEstimate string `yaml:"remainingEstimate"`
}

type MarkdownMetadata struct {
	Issuetype      string
	Project        string
	Key            string
	EpicLabelfield string `yaml:"epicLabelField"`
	EpicLabel      string `yaml:"epicLabel"`
	Summary        string
	TimeTracking   TimeTracking `yaml:"timeTracking"`
	Labels         []string
	Attachments    []string
	Depdendencies  []Depdendency `yaml:"dependencies"`
}

type Depdendency struct {
	EpicLinkfield    string `yaml:"epicLinkField"`
	DependencyType   string `yaml:"type"`
	DependencyTicket string `yaml:"ticket"`
}

type Ticket struct {
	Metadata        MarkdownMetadata
	MarkdownVersion string
	JiraVersion     string
	JiraIssue       *jira.Issue
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

func ToJiraMD(md string) string {
	if md == "" {
		return md
	}

	renderer := &cf.Renderer{Flags: cf.IgnoreMacroEscaping}
	r := bf.New(bf.WithRenderer(renderer), bf.WithExtensions(bf.CommonExtensions))

	return string(renderer.Render(r.Parse([]byte(md))))
}

func parseTicketMetadata(path string) (Ticket, error) {
	var result Ticket
	content, err := ioutil.ReadFile(path)
	if err != nil {
		return result, err
	}
	// Get rid of Windows line breaks if any
	_content := strings.ReplaceAll(string(content), "\r", "")
	body, err := particle.YAMLEncoding.DecodeString(_content, &result.Metadata)
	if err != nil {
		return result, err
	}

	result.MarkdownVersion = string(body)
	return result, nil
}

func JiraConnect(authType string, usr string, pwd string, server string) (*jira.Client, error) {
	switch authType {
	case "basic":
		tp := jira.BasicAuthTransport{
			Username: usr,
			Password: pwd,
		}
		return jira.NewClient(tp.Client(), server)
	case "token":
		tp := jira.BearerAuthTransport{
			Token: usr,
		}
		return jira.NewClient(tp.Client(), server)

	default:
		panic("Invalid auth type: " + authType)
	}
}

func SaveToJira(ticketInfo Ticket, creator string, assignee string, basePath string, jiraClient *jira.Client) (*jira.Issue, error) {
	i := jira.Issue{
		Fields: &jira.IssueFields{
			Assignee: &jira.User{
				Name: assignee,
			},
			Reporter: &jira.User{
				Name: creator,
			},
			Description: ticketInfo.JiraVersion,
			Type: jira.IssueType{
				Name: ticketInfo.Metadata.Issuetype,
			},
			Project: jira.Project{
				Key: ticketInfo.Metadata.Project,
			},
			Summary: ticketInfo.Metadata.Summary,
		},
	}
	if len(ticketInfo.Metadata.TimeTracking.RemainingEstimate) > 0 {
		i.Fields.TimeTracking = &jira.TimeTracking{
			OriginalEstimate:  ticketInfo.Metadata.TimeTracking.OriginalEstimate,
			RemainingEstimate: ticketInfo.Metadata.TimeTracking.RemainingEstimate,
		}
	}

	if len(ticketInfo.Metadata.EpicLabelfield) > 0 {
		unknowns := tcontainer.NewMarshalMap()
		unknowns[ticketInfo.Metadata.EpicLabelfield] = ticketInfo.Metadata.EpicLabel
		i.Fields.Unknowns = unknowns
	}

	i.Fields.Labels = append(i.Fields.Labels, ticketInfo.Metadata.Labels...)
	issue, resp, err := jiraClient.Issue.Create(&i)
	if err != nil {
		data, _ := ioutil.ReadAll(resp.Body)
		fmt.Println(string(data[:]))
		return issue, err
	}

	for _, a := range ticketInfo.Metadata.Attachments {
		filePath := filepath.Join(basePath, a)
		f, err := os.Open(filePath)
		if err != nil {
			fmt.Printf("Error opening file: %s\n", filePath)
		} else {
			r := bufio.NewReader(f)
			_, resp, err := jiraClient.Issue.PostAttachment(issue.Key, r, a)
			if err != nil {
				fmt.Printf("Error uploading attachment: %s\n", a)
				data, _ := ioutil.ReadAll(resp.Body)
				fmt.Println(string(data[:]))
				resp.Body.Close()
			}
			f.Close()
		}
	}

	return issue, err
}

func CreateLinks(issueMap map[string]Ticket, client *jira.Client) {
	for k, ticket := range issueMap {
		for _, d := range ticket.Metadata.Depdendencies {
			if linked, ok := issueMap[d.DependencyTicket]; ok {
				if d.DependencyType == "Epic" {
					fields := make(map[string]interface{})
					fields[d.EpicLinkfield] = linked.JiraIssue.Key
					temp := make(map[string]interface{})
					temp["fields"] = fields
					_, err := client.Issue.UpdateIssue(ticket.JiraIssue.Key, temp)
					if err != nil {
						fmt.Printf("Error linking %s to Epic %s: %s", k, d.DependencyTicket, err)
					}
				} else {
					il := &jira.IssueLink{
						Type: jira.IssueLinkType{
							Name: d.DependencyType,
						},
						InwardIssue: &jira.Issue{
							Key: ticket.JiraIssue.Key,
						},
						OutwardIssue: &jira.Issue{
							Key: linked.JiraIssue.Key,
						},
					}
					_, err := client.Issue.AddLink(il)
					if err != nil {
						fmt.Printf("Error linking %s to %s (%s): %s", k, d.DependencyTicket, d.DependencyType, err)
					}
				}
			}
		}
	}
}

func main() {

	parser := argparse.NewParser("mdToJira", "Tool to convert a set of markdown files to Jira tickets")
	u := parser.String("u", "user", &argparse.Options{Required: false, Help: "Username to create tickets with"})
	r := parser.String("r", "recipient", &argparse.Options{Required: false, Help: "Recipient of the tickets"})
	p := parser.String("p", "password", &argparse.Options{Required: false, Help: "Password for user"})
	a := parser.String("a", "auth-type", &argparse.Options{Required: true, Default: "basic", Help: "Authentication type: basic or token"})
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
	client, err := JiraConnect(*a, *u, *p, *j)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	client.User.Get(*u)
	files := readFiles(*f)

	issueMap := make(map[string]Ticket)

	for _, file := range files {
		m, err := parseTicketMetadata(file)
		if err == nil {
			m.JiraVersion = ToJiraMD(m.MarkdownVersion)
			if !*d {
				jiraIssue, e := SaveToJira(m, *u, *r, *f, client)
				if e == nil {
					m.JiraIssue = jiraIssue
					issueMap[m.Metadata.Key] = m
				} else {
					fmt.Printf("Error saving tickt %s: %s", m.Metadata.Key, err)
				}
			}
		} else {
			fmt.Println(err)
		}
	}

	CreateLinks(issueMap, client)

}
