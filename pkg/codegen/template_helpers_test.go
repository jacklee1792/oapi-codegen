package codegen

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExamplePrimaryResponseInfo(t *testing.T) {
	// Get a spec from the test definition in this file:
	swagger, err := openapi3.NewLoader().LoadFromData([]byte(testOpenAPIDefinition))
	require.NoError(t, err)

	// Get the operation definitions
	ops, err := OperationDefinitions(swagger)
	require.NoError(t, err)

	for _, op := range ops {
		switch {
		// We specified x-primary-response in the operation
		case op.Path == "/test/{name}":
			info := getPrimaryResponseInfo(&op)
			assert.Equal(t, info.statusCode, "200")
			assert.Equal(t, info.contentType, "application/xml")
			assert.ElementsMatch(t, []string{"ok"}, info.metadataProperties)
		// We did not specify x-primary-response for this operation
		case op.Path == "/cat":
			info := getPrimaryResponseInfo(&op)
			assert.Nil(t, info)
		}
	}
}

func TestExamplePrimaryResponseTypeDefinition(t *testing.T) {
	// Get a spec from the test definition in this file:
	swagger, err := openapi3.NewLoader().LoadFromData([]byte(testOpenAPIDefinition))
	require.NoError(t, err)

	// Get the operation definitions
	ops, err := OperationDefinitions(swagger)
	require.NoError(t, err)

	for _, op := range ops {
		switch {
		// We specified x-primary-response in the operation
		case op.Path == "/test/{name}":
			td := getPrimaryResponseTypeDefinition(&op)
			var propNames []string
			for _, prop := range td.Schema.Properties {
				propNames = append(propNames, prop.JsonFieldName)
			}
			assert.ElementsMatch(t, []string{"ok", "tests"}, propNames)
		// We did not specify x-primary-response for this operation
		case op.Path == "/cat":
			td := getPrimaryResponseTypeDefinition(&op)
			assert.Nil(t, td)
		}
	}
}

func TestFlattenSchema(t *testing.T) {
	// Get a spec from the test definition in this file:
	swagger, err := openapi3.NewLoader().LoadFromData([]byte(testOpenAPIDefinition))
	require.NoError(t, err)

	// Get the operation definitions
	ops, err := OperationDefinitions(swagger)
	require.NoError(t, err)

	var getTestByNameOp *OperationDefinition
	for _, op := range ops {
		if op.Path == "/test/{name}" {
			getTestByNameOp = &op
		}
	}
	require.NotNil(t, getTestByNameOp)
}
