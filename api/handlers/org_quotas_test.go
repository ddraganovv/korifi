package handlers_test

import (
	"log"
	"net/http"
	"net/url"
	"plugin"
	"strings"

	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/handlers/fake"
	"code.cloudfoundry.org/korifi/api/routing"
	. "code.cloudfoundry.org/korifi/tests/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OrgQuotas", func() {
	var (
		requestMethod string
		requestPath   string
		requestBody   string
	)

	BeforeEach(func() {
		handler := getHandler()
		routerBuilder.LoadRoutes(handler)
	})

	JustBeforeEach(func() {
		req, err := http.NewRequestWithContext(ctx, requestMethod, requestPath, strings.NewReader(requestBody))
		Expect(err).NotTo(HaveOccurred())

		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET /v3/organization_quotas/{guid}", func() {
		BeforeEach(func() {
			requestMethod = http.MethodGet
			requestPath = "/v3/organization_quotas/org-quota-guid"
			requestBody = ""
		})

		It("returns the organization quota", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))
			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.guid", "org-quota-guid"),
				MatchJSONPath("$.name", "org-quota-name"),
			)))
		})
	})
})

func getHandler() routing.Routable {
	var pluginsDir string = "/Users/C5382009/dev/korifi/open-source_korifi/korifi/api/handlers/plugins"

	plugin, err := plugin.Open(pluginsDir + "/org_quotas.so")
	if err != nil {
		log.Fatalf("plugin open: %s", err)
	}

	ctor, err := plugin.Lookup("NewPluginHandler")
	if err != nil {
		log.Fatalf("plugin Lookup: %s", err)
	}
	constructor, ok := ctor.(func(url.URL, handlers.RequestValidator) any)
	if !ok {
		log.Fatalf("constructor type assertion")
	}

	requestValidator := new(fake.RequestValidator)

	handlerObj := constructor(*serverURL, requestValidator)
	handler, ok := handlerObj.(routing.Routable)
	if !ok {
		log.Fatalf("handler assertion")
	}

	return handler
}

// go test ./... -ginkgo.focus="OrgQuotas" -v
