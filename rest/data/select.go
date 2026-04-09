package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
	testselection "github.com/evergreen-ci/test-selection-client"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// newTestSelectionClient constructs a new test selection client using the
// provided HTTP client.
func newTestSelectionClient(c *http.Client) *testselection.APIClient {
	tssBaseURL := evergreen.GetEnvironment().Settings().TestSelection.URL
	conf := testselection.NewConfiguration()
	conf.HTTPClient = c
	conf.Servers = testselection.ServerConfigurations{
		testselection.ServerConfiguration{
			URL:         tssBaseURL,
			Description: "Test selection service",
		},
	}
	return testselection.NewAPIClient(conf)
}

// SelectTests uses the test selection service to return a filtered set of tests
// to run based on the provided SelectTestsRequest. It returns the list of
// selected tests.
func SelectTests(ctx context.Context, req model.SelectTestsRequest) ([]string, error) {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	c := newTestSelectionClient(httpClient)
	var strategies []testselection.StrategyEnum
	for _, s := range req.Strategies {
		strategies = append(strategies, testselection.StrategyEnum(s))
	}

	var (
		ptrTests []*string
		resp     *http.Response
		err      error
	)
	if len(req.Tests) == 0 {
		ptrTests, resp, err = c.TestSelectionAPI.SelectAllKnownTestsOfATaskApiTestSelectionSelectKnownTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(ctx, req.Project, req.Requester, req.BuildVariant, req.TaskID, req.TaskName).StrategyEnum(strategies).Execute()
	} else {
		reqBody := testselection.BodySelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost{
			TestNames:  req.Tests,
			Strategies: strategies,
		}

		ptrTests, resp, err = c.TestSelectionAPI.SelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(ctx, req.Project, req.Requester, req.BuildVariant, req.TaskID, req.TaskName).
			BodySelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(reqBody).
			Execute()
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrap(err, "forwarding request to test selection service")
	}
	selectedTests := make([]string, 0, len(ptrTests))
	for _, t := range ptrTests {
		if t != nil {
			selectedTests = append(selectedTests, *t)
		}
	}
	return selectedTests, nil
}

// SetTestQuarantined marks the test as quarantined or unquarantined in the test
// selection service.
func SetTestQuarantined(ctx context.Context, projectID, bvName, taskName, testName string, isQuarantined bool) error {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	_, resp, err := c.StateTransitionAPI.MarkTestsAsManuallyQuarantinedApiTestSelectionTransitionTestsProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		IsManuallyQuarantined(isQuarantined).
		RequestBody([]*string{&testName}).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Wrap(err, "forwarding request to test selection service")
}

// GetTestQuarantineStatus checks whether the given test is currently
// quarantined in the test selection service using the GetTestsState API.
func GetTestQuarantineStatus(ctx context.Context, projectID, bvName, taskName, testName string) (bool, error) {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	result, resp, err := c.StateTransitionAPI.GetTestsStateApiTestSelectionGetTestsStateProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		RequestBody([]string{testName}).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return false, errors.Wrap(err, "forwarding request to test selection service")
	}
	if result == nil {
		return false, errors.New("empty response from test selection service")
	}

	stateInfo, ok := (*result)[testName]
	if !ok {
		return false, errors.Errorf("test '%s' not found in test selection service response", testName)
	}
	// Check OverrideState first since it takes precedence over the
	// computed State when set.
	if overrideState, ok := stateInfo.GetOverrideStateOk(); ok && overrideState != nil {
		return *overrideState == testselection.STATEMACHINEENUM_MANUALLY_QUARANTINED, nil
	}
	return stateInfo.State == testselection.STATEMACHINEENUM_MANUALLY_QUARANTINED, nil
}
