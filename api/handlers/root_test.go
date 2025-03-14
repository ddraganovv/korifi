package handlers_test

import (
	"net/http"
	"net/url"

	"code.cloudfoundry.org/korifi/api/config"
	"code.cloudfoundry.org/korifi/api/handlers"
	. "code.cloudfoundry.org/korifi/tests/matchers"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Root", func() {
	var (
		apiHandler  *handlers.Root
		logCacheURL *url.URL
		req         *http.Request
	)

	BeforeEach(func() {
		var err error
		logCacheURL, err = url.Parse("https://my.logcache.org")
		Expect(err).NotTo(HaveOccurred())

		apiHandler = handlers.NewRoot(*serverURL, config.UAA{}, *logCacheURL)
	})

	JustBeforeEach(func() {
		routerBuilder.LoadRoutes(apiHandler)
		routerBuilder.Build().ServeHTTP(rr, req)
	})

	Describe("GET / endpoint", func() {
		BeforeEach(func() {
			var err error
			req, err = http.NewRequest("GET", "/", nil)
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the expected response", func() {
			Expect(rr).To(HaveHTTPStatus(http.StatusOK))
			Expect(rr).To(HaveHTTPHeaderWithValue("Content-Type", "application/json"))

			Expect(rr).To(HaveHTTPBody(SatisfyAll(
				MatchJSONPath("$.cf_on_k8s", true),
				MatchJSONPath("$.links.self.href", "https://api.example.org"),
				MatchJSONPath("$.links.cloud_controller_v3.href", "https://api.example.org/v3"),
				MatchJSONPath("$.links.log_cache.href", "https://my.logcache.org"),
			)))
		})

		When("UAA support is enabled", func() {
			BeforeEach(func() {
				apiHandler = handlers.NewRoot(
					*serverURL,
					config.UAA{
						Enabled: true,
						URL:     "https://my.uaa",
					},
					*logCacheURL)
			})

			It("returns the uaa config", func() {
				Expect(rr).To(HaveHTTPStatus(http.StatusOK))

				Expect(rr).To(HaveHTTPBody(SatisfyAll(
					MatchJSONPath("$.cf_on_k8s", false),
					MatchJSONPath("$.links.uaa.href", "https://my.uaa"),
					MatchJSONPath("$.links.login.href", "https://my.uaa"),
				)))
			})
		})
	})
})
