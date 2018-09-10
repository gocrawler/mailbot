package main

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Default settings
var (
	DefaultFileName = "crawler" + strconv.FormatInt(time.Now().UnixNano(), 36) + ".log"
)

// Crawler holds the flags and locks
type Crawler struct {
	flags struct {
		filename      string
		printToStdout bool
		verbose       bool
		pastebin      bool
		debian        bool
		slexy         bool
	}
	file *os.File
}

var blacklist = []string{
	"formorer@debian.org",
	"user@user",
}

var c = new(Crawler)

func init() {
	var err error
	flag.StringVar(
		&c.flags.filename,
		"o",
		DefaultFileName,
		"File to save the collected mails",
	)
	flag.BoolVar(
		&c.flags.printToStdout,
		"stdout",
		true,
		"Print to stdout",
	)
	flag.BoolVar(
		&c.flags.verbose,
		"verbose",
		false,
		"Verbose mode",
	)
	flag.BoolVar(
		&c.flags.pastebin,
		"pastebin",
		true,
		"Crawl pastebin.com",
	)
	flag.BoolVar(
		&c.flags.debian,
		"debian",
		true,
		"Crawl paste.debian.net",
	)
	flag.BoolVar(
		&c.flags.slexy,
		"slexy",
		true,
		"Crawl slexy.org",
	)

	flag.Parse()

	c.file, err = os.OpenFile(
		c.flags.filename,
		os.O_APPEND|os.O_WRONLY|os.O_CREATE,
		0600,
	)
	if err != nil {
		report(err)
	}
}

func main() {
	c.Run()
}

// Run runs the crawler
func (c *Crawler) Run() {
	var wg = &sync.WaitGroup{}
	for {
		if c.flags.pastebin {
			wg.Add(1)
			go c.Pastebin(wg)
		}
		if c.flags.debian {
			wg.Add(1)
			go c.Debian(wg)
		}
		if c.flags.slexy {
			wg.Add(1)
			go c.Slexy(wg)
		}
		wg.Wait()
	}
}

// GetMail extracts email addresses from text documents
func (c *Crawler) GetMail(page string) {
	r := regexp.MustCompile(`[\w]+@[\w.]+`)
	mails := r.FindAllString(page, -1)
	if mails == nil {
		if c.flags.verbose {
			report(
				errors.New("no mail found"),
			)
		}
		return
	}
	fresh := FreshFilter(mails)
	if len(fresh) != 0 {
		return
	}
	toWrite := strings.Join(fresh, "\n")
	c.file.WriteString(toWrite + "\n")
	if c.flags.printToStdout {
		fmt.Println(toWrite)
	}
	c.file.Sync()
	return
}

// FetchPage fetches/scrapes pages from web URLsl
func (c *Crawler) FetchPage(url string) (string, error) {
	if c.flags.verbose {
		fmt.Printf("Fetching: %s\n", url)
	}
	client := &http.Client{}
	resp, err := client.Get(url)
	if err != nil {
		report(err)
		return "", err
	}
	b, err := ioutil.ReadAll(resp.Body)
	return string(b), err
}

// Pastebin collects emails from pastebin.com
func (c *Crawler) Pastebin(wg *sync.WaitGroup) {
	defer wg.Done()
	r := regexp.MustCompile(`class="i_p0" alt="" /><a href="(.*?)">`)
	url := "https://pastebin.com/archive"
	page, err := c.FetchPage(url)
	if err != nil {
		report(err)
	}
	raws := r.FindAllString(page, -1)
	if raws == nil {
		if c.flags.verbose {
			report(errors.New("no raw link"))
		}
		return
	}
	for _, v := range raws {
		parser := strings.Split(v, `="`)
		if len(parser) < 4 {
			report(errors.New("can't parse"))
			return
		}
		rawlink := "https://pastebin.com/raw" + strings.Replace(parser[3], `">`, "", -1)
		page, err := c.FetchPage(rawlink)
		if err != nil {
			report(err)
			return
		}
		c.GetMail(page)
	}

}

// Debian collects emails from paste.debian.net
func (c *Crawler) Debian(wg *sync.WaitGroup) {
	defer wg.Done()
	r := regexp.MustCompile(`<li><a href='//paste.debian.net(.*?)'>`)
	url := "http://paste.debian.net"
	page, err := c.FetchPage(url)
	if err != nil {
		report(err)
	}
	raws := r.FindAllString(page, -1)
	if raws == nil {
		if c.flags.verbose {
			report(errors.New("no raw link"))
		}
		return
	}
	for _, v := range raws {
		parser := strings.Split(v, `<li><a href='//`)
		if len(parser) < 2 {
			report(errors.New("can't parse"))
			return
		}
		rawlink := "http://" + strings.Replace(parser[1], `'>`, "", -1)
		page, err := c.FetchPage(rawlink)
		if err != nil {
			report(err)
			return
		}
		c.GetMail(page)
	}

}

// Slexy collects emails from slexy.org
func (c *Crawler) Slexy(wg *sync.WaitGroup) {
	defer wg.Done()
	r := regexp.MustCompile(`\/view(.*?)">`)
	url := "http://slexy.org/recent"
	page, err := c.FetchPage(url)
	if err != nil {
		report(err)
	}
	raws := r.FindAllString(page, -1)
	if raws == nil {
		if c.flags.verbose {
			report(errors.New("no raw link"))
		}
		return
	}
	for _, v := range raws {
		parser := strings.Split(v, `/view`)
		if len(parser) < 2 {
			report(errors.New("can't parse"))
			return
		}
		rawlink := "http://slexy.org/raw" + strings.Replace(parser[1], `">`, "", -1)
		page, err := c.FetchPage(rawlink)
		if err != nil {
			report(err)
			return
		}
		c.GetMail(page)
	}
}

func report(err error) {
	fmt.Fprintln(os.Stderr, err)
}

// FreshFilter filters out invalid email addresses
func FreshFilter(mails []string) []string {
	var fresh []string
	for _, mail := range mails {
		var blocked bool

		if strings.Contains(mail, ".png") {
			continue
		}
		if strings.Contains(mail, ".gif") {
			continue
		}
		if strings.Contains(mail, ".jpg") {
			continue
		}
		if strings.Contains(mail, "._") {
			continue
		}
		if strings.Contains(mail, "@.") {
			continue
		}
		if !strings.Contains(mail, ".") {
			continue
		}
		for _, black := range blacklist {
			if mail == black {
				fmt.Println(mail, black)
				blocked = true
			}
		}
		if !blocked {
			fresh = append(fresh, mail)
		}
	}
	return fresh
}
