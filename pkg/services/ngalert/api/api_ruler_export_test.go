package api

import (
	"context"
	"embed"
	"encoding/json"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/services/datasources"
	folder2 "github.com/grafana/grafana/pkg/services/folder"
	apimodels "github.com/grafana/grafana/pkg/services/ngalert/api/tooling/definitions"
	ngmodels "github.com/grafana/grafana/pkg/services/ngalert/models"
	"github.com/grafana/grafana/pkg/services/ngalert/tests/fakes"
)

//go:embed test-data/*.*
var testData embed.FS

func TestExportFromPayload(t *testing.T) {
	orgID := int64(1)
	folder := &folder2.Folder{
		UID:   "e4584834-1a87-4dff-8913-8a4748dfca79",
		Title: "foo bar",
	}

	ruleStore := fakes.NewRuleStore(t)
	ruleStore.Folders[orgID] = append(ruleStore.Folders[orgID], folder)

	srv := createService(ruleStore)

	requestFile := "post-rulegroup-101.json"
	rawBody, err := testData.ReadFile(path.Join("test-data", requestFile))
	require.NoError(t, err)
	var body apimodels.PostableRuleGroupConfig
	require.NoError(t, json.Unmarshal(rawBody, &body))

	createRequest := func() *contextmodel.ReqContext {
		return createRequestContextWithPerms(orgID, map[int64]map[string][]string{}, nil)
	}

	t.Run("accept header contains yaml, GET returns text yaml", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Header.Add("Accept", "application/yaml")

		response := srv.ExportFromPayload(rc, body, folder.Title)

		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "text/yaml", rc.Context.Resp.Header().Get("Content-Type"))
	})

	t.Run("query format contains yaml, GET returns text yaml", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Form.Set("format", "yaml")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "text/yaml", rc.Resp.Header().Get("Content-Type"))
	})

	t.Run("query format contains unknown value, GET returns text yaml", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Form.Set("format", "foo")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "text/yaml", rc.Context.Resp.Header().Get("Content-Type"))
	})

	t.Run("accept header contains json, GET returns json", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Header.Add("Accept", "application/json")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "application/json", rc.Context.Resp.Header().Get("Content-Type"))
	})

	t.Run("accept header contains json and yaml, GET returns json", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Header.Add("Accept", "application/json, application/yaml")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "application/json", rc.Context.Resp.Header().Get("Content-Type"))
	})

	t.Run("query param download=true, GET returns content disposition attachment", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Form.Set("download", "true")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Contains(t, rc.Context.Resp.Header().Get("Content-Disposition"), "attachment")
	})

	t.Run("query param download=false, GET returns empty content disposition", func(t *testing.T) {
		rc := createRequest()
		rc.Context.Req.Form.Set("download", "false")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "", rc.Context.Resp.Header().Get("Content-Disposition"))
	})

	t.Run("query param download not set, GET returns empty content disposition", func(t *testing.T) {
		rc := createRequest()

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, "", rc.Context.Resp.Header().Get("Content-Disposition"))
	})

	t.Run("json body content is as expected", func(t *testing.T) {
		expectedResponse, err := testData.ReadFile(path.Join("test-data", strings.Replace(requestFile, ".json", "-export.json", 1)))
		require.NoError(t, err)

		rc := createRequest()
		rc.Context.Req.Header.Add("Accept", "application/json")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)
		t.Log(string(response.Body()))

		require.Equal(t, 200, response.Status())
		require.JSONEq(t, string(expectedResponse), string(response.Body()))
	})

	t.Run("yaml body content is as expected", func(t *testing.T) {
		expectedResponse, err := testData.ReadFile(path.Join("test-data", strings.Replace(requestFile, ".json", "-export.yaml", 1)))
		require.NoError(t, err)

		rc := createRequest()
		rc.Context.Req.Header.Add("Accept", "application/yaml")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)
		require.Equal(t, 200, response.Status())
		require.Equal(t, string(expectedResponse), string(response.Body()))
	})

	t.Run("hcl body content is as expected", func(t *testing.T) {
		expectedResponse, err := testData.ReadFile(path.Join("test-data", strings.Replace(requestFile, ".json", "-export.hcl", 1)))
		require.NoError(t, err)

		rc := createRequest()
		rc.Context.Req.Form.Set("format", "hcl")
		rc.Context.Req.Form.Set("download", "false")

		response := srv.ExportFromPayload(rc, body, folder.Title)
		response.WriteTo(rc)

		require.Equal(t, 200, response.Status())
		require.Equal(t, string(expectedResponse), string(response.Body()))
		require.Equal(t, "text/hcl", rc.Resp.Header().Get("Content-Type"))

		t.Run("and add specific headers if download=true", func(t *testing.T) {
			rc := createRequest()
			rc.Context.Req.Form.Set("format", "hcl")
			rc.Context.Req.Form.Set("download", "true")

			response := srv.ExportFromPayload(rc, body, folder.Title)
			response.WriteTo(rc)

			require.Equal(t, 200, response.Status())
			require.Equal(t, string(expectedResponse), string(response.Body()))
			require.Equal(t, "application/terraform+hcl", rc.Resp.Header().Get("Content-Type"))
			require.Equal(t, `attachment;filename=export.tf`, rc.Resp.Header().Get("Content-Disposition"))
		})
	})
}

func TestExportRules(t *testing.T) {
	uids := sync.Map{}
	orgID := int64(1)
	f1 := randFolder()
	f2 := randFolder()

	ruleStore := fakes.NewRuleStore(t)

	hasAccessKey1 := ngmodels.AlertRuleGroupKey{
		OrgID:        orgID,
		NamespaceUID: f1.UID,
		RuleGroup:    "HAS-ACCESS-1",
	}
	accessQuery := ngmodels.GenerateAlertQuery()
	noAccessQuery := ngmodels.GenerateAlertQuery()

	_, hasAccess1 := ngmodels.GenerateUniqueAlertRules(5,
		ngmodels.AlertRuleGen(
			ngmodels.WithUniqueUID(&uids),
			withGroupKey(hasAccessKey1),
			ngmodels.WithQuery(accessQuery),
			ngmodels.WithUniqueGroupIndex(),
		))
	ruleStore.PutRule(context.Background(), hasAccess1...)
	noAccessKey1 := ngmodels.AlertRuleGroupKey{
		OrgID:        orgID,
		NamespaceUID: f1.UID,
		RuleGroup:    "NO-ACCESS",
	}
	_, noAccess1 := ngmodels.GenerateUniqueAlertRules(5,
		ngmodels.AlertRuleGen(
			ngmodels.WithUniqueUID(&uids),
			withGroupKey(noAccessKey1),
			ngmodels.WithQuery(noAccessQuery),
		))
	noAccessRule := ngmodels.AlertRuleGen(
		ngmodels.WithUniqueUID(&uids),
		withGroupKey(noAccessKey1),
		ngmodels.WithQuery(accessQuery),
	)()
	noAccess1 = append(noAccess1, noAccessRule)
	ruleStore.PutRule(context.Background(), noAccess1...)

	hasAccessKey2 := ngmodels.AlertRuleGroupKey{
		OrgID:        orgID,
		NamespaceUID: f2.UID,
		RuleGroup:    "HAS-ACCESS-2",
	}
	_, hasAccess2 := ngmodels.GenerateUniqueAlertRules(5,
		ngmodels.AlertRuleGen(
			ngmodels.WithUniqueUID(&uids),
			withGroupKey(hasAccessKey2),
			ngmodels.WithQuery(accessQuery),
			ngmodels.WithUniqueGroupIndex(),
		))
	ruleStore.PutRule(context.Background(), hasAccess2...)

	_, noAccessByFolder := ngmodels.GenerateUniqueAlertRules(10,
		ngmodels.AlertRuleGen(
			ngmodels.WithUniqueUID(&uids),
			ngmodels.WithQuery(accessQuery), // no access because of folder
			ngmodels.WithNamespaceUIDNotIn(f1.UID, f2.UID),
		))

	ruleStore.PutRule(context.Background(), noAccessByFolder...)
	// overwrite the folders visible to user because PutRule automatically creates folders in the fake store.
	ruleStore.Folders[orgID] = []*folder2.Folder{f1, f2}

	srv := createService(ruleStore)

	testCases := []struct {
		title           string
		params          url.Values
		headers         http.Header
		expectedStatus  int
		expectedHeaders http.Header
		expectedRules   []*ngmodels.AlertRule
	}{
		{
			title:          "return all rules user has access to when no parameters",
			expectedStatus: 200,
			expectedHeaders: http.Header{
				"Content-Type": []string{"text/yaml"},
			},
			expectedRules: append(hasAccess1, hasAccess2...),
		},
		{
			title: "return all rules in folder",
			params: url.Values{
				"folderUid": []string{hasAccessKey1.NamespaceUID},
			},
			expectedStatus: 200,
			expectedHeaders: http.Header{
				"Content-Type": []string{"text/yaml"},
			},
			expectedRules: hasAccess1,
		},
		{
			title: "return all rules in many folders",
			params: url.Values{
				"folderUid": []string{hasAccessKey1.NamespaceUID, hasAccessKey2.NamespaceUID},
			},
			expectedStatus: 200,
			expectedHeaders: http.Header{
				"Content-Type": []string{"text/yaml"},
			},
			expectedRules: append(hasAccess1, hasAccess2...),
		},
		{
			title: "return rules in single group",
			params: url.Values{
				"folderUid": []string{hasAccessKey1.NamespaceUID},
				"group":     []string{hasAccessKey1.RuleGroup},
			},
			expectedStatus: 200,
			expectedHeaders: http.Header{
				"Content-Type": []string{"text/yaml"},
			},
			expectedRules: hasAccess1,
		},
		{
			title: "return single rule",
			params: url.Values{
				"ruleUid": []string{hasAccess1[0].UID},
			},
			expectedStatus: 200,
			expectedHeaders: http.Header{
				"Content-Type": []string{"text/yaml"},
			},
			expectedRules: []*ngmodels.AlertRule{hasAccess1[0]},
		},
		{
			title: "fail if group and many folders",
			params: url.Values{
				"folderUid": []string{hasAccessKey1.NamespaceUID, hasAccessKey2.NamespaceUID},
				"group":     []string{hasAccessKey1.RuleGroup},
			},
			expectedStatus: 400,
		},
		{
			title: "fail if ruleUid and group",
			params: url.Values{
				"folderUid": []string{hasAccessKey1.NamespaceUID},
				"group":     []string{hasAccessKey1.RuleGroup},
				"ruleUid":   []string{hasAccess1[0].UID},
			},
			expectedStatus: 400,
		},
		{
			title: "fail if ruleUid and folderUid",
			params: url.Values{
				"folderUid": []string{hasAccessKey1.NamespaceUID},
				"ruleUid":   []string{hasAccess1[0].UID},
			},
			expectedStatus: 400,
		},
		{
			title: "unauthorized if folders are not accessible",
			params: url.Values{
				"folderUid": []string{noAccessByFolder[0].NamespaceUID},
			},
			expectedStatus: 401,
			expectedRules:  nil,
		},
		{
			title: "unauthorized if group is not accessible",
			params: url.Values{
				"folderUid": []string{noAccessKey1.NamespaceUID},
				"group":     []string{noAccessKey1.RuleGroup},
			},
			expectedStatus: 401,
		},
		{
			title: "unauthorized if rule's group is not accessible",
			params: url.Values{
				"ruleUid": []string{noAccessRule.UID},
			},
			expectedStatus: 401,
		},
		{
			title: "return in JSON if header is specified",
			headers: http.Header{
				"Accept": []string{"application/json"},
			},
			expectedStatus: 200,
			expectedRules:  append(hasAccess1, hasAccess2...),
			expectedHeaders: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		{
			title: "return in JSON if format is specified",
			params: url.Values{
				"format": []string{"json"},
			},
			expectedStatus: 200,
			expectedRules:  append(hasAccess1, hasAccess2...),
			expectedHeaders: http.Header{
				"Content-Type": []string{"application/json"},
			},
		},
		{
			title: "return in HCL if format is specified",
			params: url.Values{
				"format": []string{"hcl"},
			},
			expectedStatus: 200,
			expectedRules:  append(hasAccess1, hasAccess2...),
			expectedHeaders: http.Header{
				"Content-Type": []string{"text/hcl"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.title, func(t *testing.T) {
			rc := createRequestContextWithPerms(orgID, map[int64]map[string][]string{
				orgID: {
					datasources.ActionQuery: []string{datasources.ScopeProvider.GetResourceScopeUID(accessQuery.DatasourceUID)},
				},
			}, nil)
			rc.Req.Form = tc.params
			rc.Req.Header = tc.headers

			resp := srv.ExportRules(rc)

			require.Equal(t, tc.expectedStatus, resp.Status())
			if tc.expectedStatus != 200 {
				return
			}
			var exp []ngmodels.AlertRuleGroupWithFolderTitle
			gr := ngmodels.GroupByAlertRuleGroupKey(tc.expectedRules)
			for key, rules := range gr {
				folder, err := ruleStore.GetNamespaceByUID(context.Background(), key.NamespaceUID, orgID, nil)
				require.NoError(t, err)
				exp = append(exp, ngmodels.NewAlertRuleGroupWithFolderTitleFromRulesGroup(key, rules, folder.Title))
			}
			sort.SliceStable(exp, func(i, j int) bool {
				gi, gj := exp[i], exp[j]
				if gi.OrgID != gj.OrgID {
					return gi.OrgID < gj.OrgID
				}
				if gi.FolderUID != gj.FolderUID {
					return gi.FolderUID < gj.FolderUID
				}
				return gi.Title < gj.Title
			})
			groups, err := AlertingFileExportFromAlertRuleGroupWithFolderTitle(exp)
			require.NoError(t, err)

			require.Equal(t, string(exportResponse(rc, groups).Body()), string(resp.Body()))

			resp.WriteTo(rc)
			actualHeaders := rc.Resp.Header()
			for h, hv := range tc.expectedHeaders {
				assert.Contains(t, actualHeaders, h)
				actual := actualHeaders[h]
				require.Equal(t, hv, actual)
			}
		})
	}
}
