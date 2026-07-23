package main

import (
	"errors"
	"strings"

	"github.com/microsoftgraph/msgraph-sdk-go/models/odataerrors"
)

func isGraphNotFound(err error) bool {
	var graphErr *odataerrors.ODataError
	if !errors.As(err, &graphErr) || graphErr.GetErrorEscaped() == nil || graphErr.GetErrorEscaped().GetCode() == nil {
		return false
	}
	code := strings.ToLower(*graphErr.GetErrorEscaped().GetCode())
	return code == "itemnotfound" || code == "notfound" || code == "resourcenotfound"
}
