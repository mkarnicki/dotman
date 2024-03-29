
// Install view - shows install menu, and executes install upon chosen options
// =================================================

package views

import (
    "strings"
	"net/http"
    "text/template"
)

type InstallData struct {
    Logo string
    ClientSecret string
    FoldersMap map[string]string
    AvailableOpts string
    RepoOpts string
    BaseURL string
    URLMask string
}

func ServeInstall(w http.ResponseWriter, r *http.Request, baseurl string, client_secret string, logo string, directory string, alphabet string, foldersMap map[string]string, urlMask string) {

    // variable for holding avilable oprions
    menuItems := make(map[string]string)

    // iterate over options
    for i := 0; i < len(alphabet); i++ {

            // exit if no keys in map anymore
            key := string(alphabet[i])
            if _, ok := foldersMap[key]; !ok {
                continue
            }

            // add item to map
            menuItems[key] = foldersMap[key]
    }

    // create variable with available options for cross checking input
    keys := make([]string, 0)
    for key := range foldersMap {
        keys = append(keys, key)
    }
    availableOpts := strings.Join(keys, "")

    // generate body of bash case with repo packages
    repoPackages := repoPackagesCasePrint(foldersMap, false, directory, baseurl)

    // build data for template
    data := InstallData{strings.ReplaceAll(logo,"'","'\"'\"'"), client_secret, menuItems, availableOpts, repoPackages, baseurl, urlMask}

    // adding functions for template
    funcMap := template.FuncMap{

        // implement increment
        "inc": func(i int) int {
            return i + 1
        },

        // modulo function
        "mod": func(i, j int) bool {
            return i%j == 0
        },
    }

    // render template
    tmpl, err := template.New("install").Funcs(funcMap).Parse(tmplInstall)
    if err != nil { panic(err) }
    err = tmpl.Execute(w, data)
    if err != nil { panic(err) }
}

var tmplInstall = bashTemplHead + gitCloneTmpl + `
tput clear
echo -e '{{.Logo}}'
barPrint
printf "%2s%s\n%2s%s\e[32m%s\e[0m%s\n\n" "" "Choose dotfiles to be installed." "" "Select by typing keys (" "green" ") and confirm with enter."
barPrint

{{ $index := 0 }}
{{ range $key, $value := .FoldersMap }}
printf "  \e[32m%s\e[0m)\e[35m %-15s\e[0m" "{{ $key }} " "{{ $value }}"
{{ $index = inc $index }}
{{ if mod $index 3 }}echo ""{{ end }}
{{ end }}

SECRET="{{ .ClientSecret }}"
selectPackage() {
    case "$1" in
    {{ .RepoOpts }}
    esac
}

OPTS="{{ .AvailableOpts }}"

exec 3<>/dev/tty
echo ""
read -u 3 -p "  Chosen packages: " words
echo ""
if [ -z $words ]; then
    echo -e "  Nothing to do... exiting."
    exit 0
fi
barPrint

echo -ne "  Follwing dotfiles will be installed in order:\n  "
COMMA=""
for CHAR in $(echo "$words" | fold -w1); do
    test "${OPTS#*$CHAR}" != "$OPTS" || continue
    echo -en "$COMMA" 
    selectPackage $CHAR False
    COMMA=", "
done

if [ "$COMMA" == "" ]; then
    echo -e "\n  Nothing to do... exiting."
    exit 0
fi

GITINSTALL=false

if [ -f "$HOME/.dotman/managed" ]; then
    if [ -d "$HOME/.dotman/dotfiles" ]; then
        GITINSTALL=true
        echo -e "\n\n  \e[33;5mWarning!\e[0m\n  Git install method used.\n  This will update any other dotfiles managed by dotman."
    fi
else
    if command -v git >/dev/null 2>&1; then
        echo -e  "\n\n  Fresh install. GIT command present. Install using git symlink method? [Y/n]"
        read -u 3 -n 1 -r -s
        [[ ! $REPLY =~ ^[Nn]$ ]] && GITINSTALL=true
    fi
fi

confirmPrompt

mkdir -p "$HOME/.dotman"; touch "$HOME/.dotman/managed"

if command -v git >/dev/null 2>&1; then
    "$GITINSTALL" && mkdir -p "$HOME/.dotman/dotfiles" || rm -rf "$HOME/.dotman/dotfiles"
fi

barPrint
echo "  Installing dotfiles:"

if [ -d "$HOME/.dotman/dotfiles" ]; then 
    gitCloneIfPresent "$SECRET"
fi

for CHAR in $(echo "$words" | fold -w1); do
    test "${OPTS#*$CHAR}" != "$OPTS" || continue
    selectPackage $CHAR 
done
`
