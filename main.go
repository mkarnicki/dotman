package main

// go get -u gopkg.in/src-d/go-git.v4/
// go get github.com/gliderlabs/ssh
// go get github.com/namsral/flag

import (
	"fmt"
    "strconv"
    "log"
    "regexp"
    "bytes"
    "io"
    "os"
    "net/http"
    "strings"
    "io/ioutil"
    "github.com/namsral/flag"
    "gopkg.in/src-d/go-git.v4/plumbing/transport"
    "cz0.cz/czoczo/dotman/views"
)


// Global variables
// =================================================

// define ssh or http connection string
var url string
var username string
var password string
var directory string
var secret string
var port int

// server config
// URL (e.g. https://exmaple.org:1338/dotfiles) under to create links to resources
var baseurl string
var urlMask string

// set of characters to assign packages to
var alphabet string
// tags map
var tags map[string]string

var ssh_known_hosts string

// authorization object for git connection
var auth transport.AuthMethod



// Program starts here
// =================================================

func main() {

    // hello
    log.Println("Starting \"dot file manager\" aka:")

    // print logo (remove colors)
    reg, _ := regexp.Compile("\\\\e\\[[0-9](;[0-9]+)?m")
    //reg, _ = regexp.Compile("\\\\e\\[")
    log.Println(reg.ReplaceAllString(getLogo(),""))

    // parse arguments/environment configuration
    var sshkey string
    var sshAccept bool
    flag.StringVar(&url, "url", "", "git repository to connect to")
    flag.StringVar(&password, "password", "", "used to connect to git repository, when using http protocol")
    flag.StringVar(&directory, "directory", "dotfiles", "endpoint under which to serve files.")
    flag.StringVar(&secret, "secret", "", "used to protect files served by server")
    flag.StringVar(&sshkey, "sshkey", "ssh_data/id_rsa", "path to key used to connect git repository when using ssh protocol.")
    flag.BoolVar(&sshAccept, "sshaccept", false, "whether to add ssh remote servers key to known hosts file")
    flag.IntVar(&port, "port", 1338, "servers listening port")
    flag.StringVar(&urlMask, "urlmask", "", "mask git repository URL in local repo (only git install method)")
    flag.StringVar(&baseurl, "baseurl", "http://127.0.0.1:1338", "URL for generating download commands.")
	flag.Parse()

    // validate configuration
    isSsh, _ := regexp.Compile("ssh://")

    // check url
    // if url variable not set
    if url == "" {
        log.Println("For server to start you must provide git repository URL containing your dotfiles in folders.")
        log.Println("Use -url switch or URL environment variable.")
        os.Exit(1)
    }

    // if url not valid
    re := regexp.MustCompile("(ssh|https?)://([^/$]+)(.+)")
    urlMatch := re.FindStringSubmatch(url)
    if re.MatchString(url) == false {
        flag.PrintDefaults()
        fmt.Println()
        log.Println("Provided repository URL: " + url + " not supported. Provide either ssh or http(s) protocol URL. Exiting.")
        os.Exit(1)
    }


    // get URL without protocol prefix for git clone operation
    remoteHostSSH := urlMatch[2]

    // extract username
    re = regexp.MustCompile("(ssh|https?)://(.+)@([^/$]+)")
    urlMatch = re.FindStringSubmatch(url)
    if len(urlMatch) > 2 {
        username = urlMatch[2]
    }

    match, _ := regexp.MatchString(".+@.+$",remoteHostSSH)
    if match == true {
        remoteHostSSH = strings.Split(remoteHostSSH,"@")[1]
    }
    match, _ = regexp.MatchString(".+:[0-9]+$",remoteHostSSH)
    if match == false {
        remoteHostSSH = strings.Split(remoteHostSSH,":")[0] + ":22"
    }
    // check baseurl
    match, _ = regexp.MatchString("https?://.+",baseurl)
    if match == false {
        log.Println("Unsupported base URL given. Use http or https protocol based URL. Exiting.")
        os.Exit(2)
    }

    // server config
    alphabet = "0123456789abcdefghijklmnopqrstuwxyzABCDEFGHIJKLMNOPQRSTUWXYZ"
    ssh_known_hosts = "ssh_data/known_hosts"

    // available packages list
    var foldersMap map[string]string


    // check if ssh protocol
    if isSsh.MatchString(url) {
        // create ssh_data dir
        os.MkdirAll("ssh_data", os.ModePerm)

        if !fileExists(sshkey) {
            log.Println("SSH Key " + sshkey + " not found. Falling back to generating key pair")
            err := MakeSSHKeyPair("ssh_data/id_rsa.pub","ssh_data/id_rsa")
            CheckIfError(err)
            log.Println("SSH Key pair generated successfully")
        }

        if !fileExists(ssh_known_hosts) {
            log.Println("SSH " + ssh_known_hosts + " file not found.")
            emptyFile, err := os.Create(ssh_known_hosts)
            CheckIfError(err)
            emptyFile.Close()
            log.Println("SSH known_hosts file successfully")
        }
    }

    // print hello and server configuration
    log.Println("Starting dotman - dot file manager.")
    log.Println("Repository URL: " + url)
    log.Println("GIT username: " + username)
    log.Println("Listening port: " + strconv.Itoa(port))
    log.Println("Download URLs prefix: " + baseurl+"/"+directory)

    // if using generated key pair print public key
    if sshkey == "ssh_data/id_rsa" && fileExists("ssh_data/id_rsa.pub") {
        log.Println("Using generate ssh key pair. Public key is:\n")
        pubkey, err := ioutil.ReadFile("ssh_data/id_rsa.pub")
        CheckIfError(err)
        fmt.Println(string(pubkey))
    }


    // sync file server directory with remote git repository

    // obatin auth object
    log.Println("remoteHost: " + remoteHostSSH)
    auth = getAuth(url, sshkey, remoteHostSSH, sshAccept)

    // do actual sync
    gitSync(auth, url, directory)

    // retrive repository folders mapped with alphabet characters
    foldersMap = getFoldersMap(directory, alphabet)

    // populate tags
    populateTagsMap(foldersMap)

    // serve locally cloned repo with dotfiles through HTTP
    // secured with secret and file blacklisting
    fs := checkSecretThenServe(http.FileServer(fileHidingFileSystem{http.Dir(directory)}))

    // handle being served as a subfolder 
    basere := regexp.MustCompile("^https?://[^/]+(.+)?")
    basematch := basere.FindStringSubmatch(baseurl)
    folder := strings.TrimSuffix(basematch[1],"/")

// URL Router, methods for handling endpoints
// =================================================

    // handle fixed routes first
    // handle file serving under 'directory' variable name
    http.Handle(folder+"/"+directory+"/", http.StripPrefix(folder+"/"+directory+"/", fs))
    log.Println("Serving files under: " + folder+"/"+directory+"/")

    // handle all other HTTP requests 
    http.HandleFunc("/", func (w http.ResponseWriter, r *http.Request) {

        // on each request:
        // log
		log.Println(r.RemoteAddr + ": " + r.RequestURI)
        // strip backslash at the end of request
        requestPath := strings.TrimSuffix(r.URL.Path,"/")

        // handle main request, print main menu script
        if requestPath == folder {
            views.ServeMain(w, r, baseurl, secret, getLogo())
            return
        }

        // handle tags endpoint
        commaListRegex, _ := regexp.Compile("^/t/([0-9a-zA-Z]+,?)+$")
        if commaListRegex.MatchString(requestPath) {
            views.ServeTags(w, r, baseurl, secret, getLogo(), directory, foldersMap, tagsData.Tags)
            return
        }

        // all other futher routes require secret 
        client_secret := r.Header.Get("secret")
        if client_secret != secret {
            log.Println("Wrong secret, or not given.")
		    fmt.Fprintf(w, "echo \"  Wrong secret, or not given.\"")
            return
        }

        // handle install endpointm print install menu script
        if requestPath == folder + "/install" {
            views.ServeInstall(w, r, baseurl, client_secret, getLogo(), directory, alphabet, foldersMap, urlMask)
            return
        }

        // handle synchronization endpoint - pull git repo
        if requestPath == folder + "/sync" {
            commit := gitPull(directory)
            fmt.Fprintf(w, "echo -e \"\\n" + commit + "\"\n")
            fmt.Fprintf(w, "echo -e \"\\n  Repository synchronised.\"")
            // populate tags
            populateTagsMap(foldersMap)
            return
        }

        // handle tags list
        if requestPath == folder + "/tagslist" {
            if len(tagsData.Tags) == 0 {
                fmt.Fprintf(w, "echo -e \"\\n  No tags configured.\\n\"\n")
                return
            }
            fmt.Fprintf(w, "echo -e \"\\n  Available tags:\\n\"\n")
            for key, val := range tagsData.Tags {
                packages := strings.Join(val[:],`\e[0m, \e[35m`)
                fmt.Fprintf(w, `echo -e "  \e[32m`+key+`\e[0m:\e[35m `+packages+" \\e[0m\"\n")
            }
            fmt.Fprintf(w, "echo -e \"\\n  To install packages use: "+baseurl+"/t/\\e[32mtagname\\e[0m\\n\"\n")
            return
        }

        // handle update script endpoint
        if requestPath == folder + "/update" {
            views.ServeUpdate(w, r, baseurl, client_secret, directory, foldersMap, urlMask)
            return
        }

        // handle change of install method script endpoint
        if requestPath == folder + "/changeInstallMethod" {
            views.ServeChangeInstallMethod(w, r, baseurl, client_secret, directory, foldersMap, urlMask)
            return
        }

        // handle auto update enable endpoint
        if requestPath == folder + "/autoenable" {
            views.ServeSetAuto(w, r, baseurl, client_secret, true)
            return
        }

        // handle auto update disable endpoint
        if requestPath == folder + "/autodisable" {
            views.ServeSetAuto(w, r, baseurl, client_secret, false)
            return
        }

        // handle download whole repository
        repoFilename := "dotfilesrepo.tar.gz"
        if requestPath == folder + "/" + repoFilename {
            // delete file if exists
            if fileExists(repoFilename) {
                err := os.Remove(repoFilename)
                if err != nil {
                    log.Fatal(err)
                }
            }
            // tar + gzip
            var buf bytes.Buffer
            _ = compress(directory, &buf)

            // write the .tar.gzip
            fileToWrite, err := os.OpenFile(repoFilename, os.O_CREATE|os.O_RDWR, os.FileMode(0600))
            if err != nil {
                panic(err)
            }
            if _, err := io.Copy(fileToWrite, &buf); err != nil {
                panic(err)
            }

            //Check if file exists and open
            Openfile, err := os.Open(repoFilename)
            defer Openfile.Close() //Close after function return
            if err != nil {
                //File not found, send 404
                http.Error(w, "File not found.", 404)
                return
            }

            //File is found, create and send the correct headers

            //Get the Content-Type of the file
            //Create a buffer to store the header of the file in
            FileHeader := make([]byte, 512)
            //Copy the headers into the FileHeader buffer
            Openfile.Read(FileHeader)
            //Get content type of file
            FileContentType := http.DetectContentType(FileHeader)

            //Get the file size
            FileStat, _ := Openfile.Stat()                     //Get info from file
            FileSize := strconv.FormatInt(FileStat.Size(), 10) //Get file size as a string

            //Send the headers
            w.Header().Set("Content-Disposition", "attachment; filename="+repoFilename)
            w.Header().Set("Content-Type", FileContentType)
            w.Header().Set("Content-Length", FileSize)

            //Send the file
            //We read 512 bytes from the file already, so we reset the offset back to 0
            Openfile.Seek(0, 0)
            io.Copy(w, Openfile) //'Copy' the file to the client
            return
        }


        // if none above catched, return 404
        w.WriteHeader(http.StatusNotFound)
        fmt.Fprintf(w, "echo \"404 - Not Found\"")
	})

	http.ListenAndServe(":"+strconv.Itoa(port), nil)
}
