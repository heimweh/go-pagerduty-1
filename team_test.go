package pagerduty

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"text/template"
)

func userID(offset, index int) int {
	return offset + index
}

func lastIndex(length, index int) bool {
	return length-1 == index
}

const membersResponseTemplate = `
{
    {{template "pageInfo" . }},
    "members": [
        {{- $length := len .roles -}}
        {{- $offset := .offset -}}
        {{- range $index, $role := .roles -}}
        {
            "user": {
                "id": "ID{{userID $offset $index}}"
            },
            "role": "{{ $role }}"
        }
        {{- if not (lastIndex $length $index) }},
        {{end -}}
        {{- end }}
    ]
}
`

var memberPageTemplate = template.Must(pageTemplate.New("membersResponse").
	Funcs(templateUtilityFuncs).
	Parse(membersResponseTemplate))

const (
	testValidTeamID = "MYTEAM"
	testAPIKey      = "MYKEY"
	testBadURL      = "A-FAKE-URL"
	testMaxPageSize = 3
)

var templateUtilityFuncs = template.FuncMap{
	"lastIndex": lastIndex,
	"userID":    userID,
}

var pageTemplate = template.Must(template.New("pageInfo").Parse(`
    "more": {{- .more -}},
    "limit": {{- .limit -}},
    "offset": {{- .offset -}}
`))

type pageDetails struct {
	lowNumber, highNumber, limit, offset int
	more                                 bool
}

func genMembersRespPage(details pageDetails, t *testing.T) string {
	if details.limit == 0 {
		details.limit = 25 // Default to 25, PD's API default.
	}

	possibleRoles := []string{"manager", "responder", "observer"}
	roles := make([]string, 0)
	for ; details.lowNumber <= details.highNumber; details.lowNumber++ {
		roles = append(roles, possibleRoles[details.lowNumber%len(possibleRoles)])
	}

	buffer := bytes.NewBufferString("")
	err := memberPageTemplate.Execute(buffer, map[string]interface{}{
		"roles":  roles,
		"more":   details.more,
		"limit":  details.limit,
		"offset": details.offset,
	})

	if err != nil {
		t.Fatalf("Failed to apply values to template: %v", err)
	}

	return string(buffer.String())
}

func genRespPages(amount,
	maxPageSize int,
	pageGenerator func(pageDetails, *testing.T) string,
	t *testing.T) []string {

	pages := make([]string, 0)

	lowNumber := 1
	offset := 0
	more := true

	for {
		tempHighNumber := amount

		if lowNumber+(maxPageSize-1) < amount {
			// Still more pages to come, this page doesn't hit upper.
			tempHighNumber = lowNumber + (maxPageSize - 1)
		} else {
			// Last page, with at least 1 user.
			more = false
		}

		// Generate page using current lower and upper.
		page := pageGenerator(pageDetails{
			lowNumber:  lowNumber,
			highNumber: tempHighNumber,
			limit:      maxPageSize,
			more:       more,
			offset:     offset}, t)

		pages = append(pages, page)

		if !more {
			// Hit the last page, stop.
			return pages
		}
		// Move the offset and lower up to prepare for next page.
		offset += maxPageSize
		lowNumber += maxPageSize
	}
}

func TestListMembersSuccess(t *testing.T) {
	expectedNumResults := testMaxPageSize - 1
	page := genRespPages(expectedNumResults, testMaxPageSize, genMembersRespPage, t)[0]

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	mux.HandleFunc("/teams/"+testValidTeamID+"/members", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, page)
	})

	api := &Client{apiEndpoint: server.URL, authToken: testAPIKey, HTTPClient: defaultHTTPClient}
	members, err := api.ListMembers(testValidTeamID, ListMembersOptions{})
	if err != nil {
		t.Fatalf("Failed to get members: %v", err)
	}

	if len(members.Members) != expectedNumResults {
		t.Fatalf("Expected %d team members, got: %v", expectedNumResults, err)
	}
}

func TestListMembersError(t *testing.T) {
	api := &Client{apiEndpoint: testBadURL, authToken: testAPIKey, HTTPClient: defaultHTTPClient}
	members, err := api.ListMembers(testValidTeamID, ListMembersOptions{})
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
	if members != nil {
		t.Fatalf("Expected nil members response, got: %v", members)
	}
}
