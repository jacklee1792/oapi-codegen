// Copyright 2019 DeepMap, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package codegen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
)

const (
	// These allow the case statements to be sorted later:
	prefixMostSpecific, prefixLessSpecific, prefixLeastSpecific = "3", "6", "9"
)

var (
	contentTypesJSON = []string{echo.MIMEApplicationJSON, "text/x-json"}
	contentTypesYAML = []string{"application/yaml", "application/x-yaml", "text/yaml", "text/x-yaml"}
	contentTypesXML  = []string{echo.MIMEApplicationXML, echo.MIMETextXML}

	responseTypeSuffix = "Response"
)

// This function takes an array of Parameter definition, and generates a valid
// Go parameter declaration from them, eg:
// ", foo int, bar string, baz float32". The preceding comma is there to save
// a lot of work in the template engine.
func genParamArgs(params []ParameterDefinition) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, len(params))
	for i, p := range params {
		paramName := p.GoVariableName()
		parts[i] = fmt.Sprintf("%s %s", paramName, p.TypeDef())
	}
	return ", " + strings.Join(parts, ", ")
}

// This function is much like the one above, except it only produces the
// types of the parameters for a type declaration. It would produce this
// from the same input as above:
// ", int, string, float32".
func genParamTypes(params []ParameterDefinition) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.TypeDef()
	}
	return ", " + strings.Join(parts, ", ")
}

// This is another variation of the function above which generates only the
// parameter names:
// ", foo, bar, baz"
func genParamNames(params []ParameterDefinition) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, len(params))
	for i, p := range params {
		parts[i] = p.GoVariableName()
	}
	return ", " + strings.Join(parts, ", ")
}

// genResponsePayload generates the payload returned at the end of each client request function
func genResponsePayload(operationID string) string {
	var buffer = bytes.NewBufferString("")

	// Here is where we build up a response:
	fmt.Fprintf(buffer, "&%s{\n", genResponseTypeName(operationID))
	fmt.Fprintf(buffer, "Body: bodyBytes,\n")
	fmt.Fprintf(buffer, "HTTPResponse: rsp,\n")
	fmt.Fprintf(buffer, "}")

	return buffer.String()
}

// genResponseUnmarshal generates unmarshaling steps for structured response payloads
func genResponseUnmarshal(op *OperationDefinition) string {
	var handledCaseClauses = make(map[string]string)
	var unhandledCaseClauses = make(map[string]string)

	// Get the type definitions from the operation:
	typeDefinitions, err := op.GetResponseTypeDefinitions()
	if err != nil {
		panic(err)
	}

	if len(typeDefinitions) == 0 {
		// No types.
		return ""
	}

	// Add a case for each possible response:
	buffer := new(bytes.Buffer)
	responses := op.Spec.Responses
	for _, typeDefinition := range typeDefinitions {

		responseRef, ok := responses[typeDefinition.ResponseName]
		if !ok {
			continue
		}

		// We can't do much without a value:
		if responseRef.Value == nil {
			fmt.Fprintf(os.Stderr, "Response %s.%s has nil value\n", op.OperationId, typeDefinition.ResponseName)
			continue
		}

		// If there is no content-type then we have no unmarshaling to do:
		if len(responseRef.Value.Content) == 0 {
			caseAction := "break // No content-type"
			caseClauseKey := "case " + getConditionOfResponseName("rsp.StatusCode", typeDefinition.ResponseName) + ":"
			unhandledCaseClauses[prefixLeastSpecific+caseClauseKey] = fmt.Sprintf("%s\n%s\n", caseClauseKey, caseAction)
			continue
		}

		// If we made it this far then we need to handle unmarshaling for each content-type:
		sortedContentKeys := SortedContentKeys(responseRef.Value.Content)
		for _, contentTypeName := range sortedContentKeys {

			// We get "interface{}" when using "anyOf" or "oneOf" (which doesn't work with Go types):
			if typeDefinition.TypeName == "interface{}" {
				// Unable to unmarshal this, so we leave it out:
				continue
			}

			// Add content-types here (json / yaml / xml etc):
			switch {

			// JSON:
			case StringInArray(contentTypeName, contentTypesJSON):
				if typeDefinition.ContentTypeName == contentTypeName {
					caseAction := fmt.Sprintf("var dest %s\n"+
						"if err := json.Unmarshal(bodyBytes, &dest); err != nil { \n"+
						" return nil, err \n"+
						"}\n"+
						"response.%s = &dest",
						typeDefinition.Schema.TypeDecl(),
						typeDefinition.TypeName)

					caseKey, caseClause := buildUnmarshalCase(typeDefinition, caseAction, "json")
					handledCaseClauses[caseKey] = caseClause
				}

			// YAML:
			case StringInArray(contentTypeName, contentTypesYAML):
				if typeDefinition.ContentTypeName == contentTypeName {
					caseAction := fmt.Sprintf("var dest %s\n"+
						"if err := yaml.Unmarshal(bodyBytes, &dest); err != nil { \n"+
						" return nil, err \n"+
						"}\n"+
						"response.%s = &dest",
						typeDefinition.Schema.TypeDecl(),
						typeDefinition.TypeName)
					caseKey, caseClause := buildUnmarshalCase(typeDefinition, caseAction, "yaml")
					handledCaseClauses[caseKey] = caseClause
				}

			// XML:
			case StringInArray(contentTypeName, contentTypesXML):
				if typeDefinition.ContentTypeName == contentTypeName {
					caseAction := fmt.Sprintf("var dest %s\n"+
						"if err := xml.Unmarshal(bodyBytes, &dest); err != nil { \n"+
						" return nil, err \n"+
						"}\n"+
						"response.%s = &dest",
						typeDefinition.Schema.TypeDecl(),
						typeDefinition.TypeName)
					caseKey, caseClause := buildUnmarshalCase(typeDefinition, caseAction, "xml")
					handledCaseClauses[caseKey] = caseClause
				}

			// Everything else:
			default:
				caseAction := fmt.Sprintf("// Content-type (%s) unsupported", contentTypeName)
				caseClauseKey := "case " + getConditionOfResponseName("rsp.StatusCode", typeDefinition.ResponseName) + ":"
				unhandledCaseClauses[prefixLeastSpecific+caseClauseKey] = fmt.Sprintf("%s\n%s\n", caseClauseKey, caseAction)
			}
		}
	}

	if len(handledCaseClauses)+len(unhandledCaseClauses) == 0 {
		// switch would be empty.
		return ""
	}

	// Now build the switch statement in order of most-to-least specific:
	// See: https://github.com/deepmap/oapi-codegen/issues/127 for why we handle this in two separate
	// groups.
	fmt.Fprintf(buffer, "switch {\n")
	for _, caseClauseKey := range SortedStringKeys(handledCaseClauses) {

		fmt.Fprintf(buffer, "%s\n", handledCaseClauses[caseClauseKey])
	}
	for _, caseClauseKey := range SortedStringKeys(unhandledCaseClauses) {

		fmt.Fprintf(buffer, "%s\n", unhandledCaseClauses[caseClauseKey])
	}
	fmt.Fprintf(buffer, "}\n")

	return buffer.String()
}

// buildUnmarshalCase builds an unmarshalling case clause for different content-types:
func buildUnmarshalCase(typeDefinition ResponseTypeDefinition, caseAction string, contentType string) (caseKey string, caseClause string) {
	caseKey = fmt.Sprintf("%s.%s.%s", prefixLeastSpecific, contentType, typeDefinition.ResponseName)
	caseClauseKey := getConditionOfResponseName("rsp.StatusCode", typeDefinition.ResponseName)
	caseClause = fmt.Sprintf("case strings.Contains(rsp.Header.Get(\"%s\"), \"%s\") && %s:\n%s\n", echo.HeaderContentType, contentType, caseClauseKey, caseAction)
	return caseKey, caseClause
}

// genResponseTypeName creates the name of generated response types (given the operationID):
func genResponseTypeName(operationID string) string {
	return fmt.Sprintf("%s%s", UppercaseFirstCharacter(operationID), responseTypeSuffix)
}

func getResponseTypeDefinitions(op *OperationDefinition) []ResponseTypeDefinition {
	td, err := op.GetResponseTypeDefinitions()
	if err != nil {
		panic(err)
	}
	return td
}

// Return the statusCode comparison clause from the response name.
func getConditionOfResponseName(statusCodeVar, responseName string) string {
	switch responseName {
	case "default":
		return "true"
	case "1XX", "2XX", "3XX", "4XX", "5XX":
		return fmt.Sprintf("%s / 100 == %s", statusCodeVar, responseName[:1])
	default:
		return fmt.Sprintf("%s == %s", statusCodeVar, responseName)
	}
}

// This outputs a string array
func toStringArray(sarr []string) string {
	return `[]string{"` + strings.Join(sarr, `","`) + `"}`
}

func stripNewLines(s string) string {
	r := strings.NewReplacer("\n", "")
	return r.Replace(s)
}

type primaryResponseInfo struct {
	statusCode         string
	contentType        string
	metadataProperties []string
}

// getPrimaryResponseInfo gets the x-primary-response extension data from the OperationDefinition.
func getPrimaryResponseInfo(op *OperationDefinition) *primaryResponseInfo {
	// Find the x-primary-response field. This is located in the top level of the
	// OperationDefinition.
	msg, ok := op.Spec.Extensions["x-primary-response"].(json.RawMessage)
	if !ok {
		return nil
	}
	m := make(map[string]interface{})
	if err := json.Unmarshal(msg, &m); err != nil {
		panic(err)
	}
	info := &primaryResponseInfo{}
	// Get status-code from x-primary-response.
	if tmp, ok := m["status-code"]; !ok {
		panic("no status-code key in x-primary-response")
	} else if info.statusCode, ok = tmp.(string); !ok {
		panic(fmt.Sprintf(
			"expected string for status-code in x-primary-response, got %T",
			tmp,
		))
	}
	// Get content-type from x-primary-response.
	if tmp, ok := m["content-type"]; !ok {
		panic("no content-type key in x-primary-response")
	} else if info.contentType, ok = tmp.(string); !ok {
		panic(fmt.Sprintf(
			"expected string for content-type in x-primary-response, got %T",
			tmp,
		))
	}
	// Get metadata-properties from x-primary-response.
	if tmp, ok := m["metadata-properties"]; !ok {
		// It's ok not to have this property.
		return info
	} else if props, ok := tmp.([]interface{}); !ok {
		panic(fmt.Sprintf(
			"expected []interface{} for metadata-properties in x-primary-response, got %T",
			tmp,
		))
	} else {
		for i, propAny := range props {
			prop, ok := propAny.(string)
			if !ok {
				panic(fmt.Sprintf(
					"expected string for metadata-properties element %v, got %T",
					i,
					propAny,
				))
			}
			info.metadataProperties = append(info.metadataProperties, prop)
		}
	}

	return info
}

// getPrimaryResponseTypeDefinition inspects the metadata on the OperationDefinition and returns
// the corresponding primary ResponseTypeDefinition if it exists. If it does not exist, it returns
// nil.
func getPrimaryResponseTypeDefinition(op *OperationDefinition) *ResponseTypeDefinition {
	info := getPrimaryResponseInfo(op)
	if info == nil {
		return nil
	}
	for _, td := range getResponseTypeDefinitions(op) {
		if td.ResponseName == info.statusCode && td.ContentTypeName == info.contentType {
			return &td
		}
	}
	panic("no match found for primary response")
}

func stringSliceContains(haystack []string, needle string) bool {
	for _, element := range haystack {
		if element == needle {
			return true
		}
	}
	return false
}

// getSingleNonMetadataProperty tries to get the single property in the TypeDefinition that
// was not marked as metadata by the x-primary-response extension. It returns the property if
// there is exactly one such property, otherwise it returns nil.
func getSingleNonMetadataProperty(td *TypeDefinition, info *primaryResponseInfo) *Property {
	var ret *Property
	for _, prop := range td.Schema.Properties {
		if !stringSliceContains(info.metadataProperties, prop.JsonFieldName) {
			if ret != nil {
				return nil
			}
			ret = &prop
		}
	}
	return ret
}

// isFlatTypeDefinition checks if the property count on the TypeDefinition is 0, meaning that it's
// a "flat" type like integer or string.
func isFlatTypeDefinition(td *TypeDefinition) bool {
	return len(td.Schema.Properties) == 0
}

// asReducedTypeDefinition makes a copy of the existing TypeDefinition where properties marked as
// metadata are purged from the Schema. If all properties but one are purged, then the result
// is flattened.
func asReducedTypeDefinition(td ResponseTypeDefinition, info *primaryResponseInfo) *TypeDefinition {
	// Try to reduce it to a single flat type definition
	if prop := getSingleNonMetadataProperty(&td.TypeDefinition, info); prop != nil {
		return &TypeDefinition{
			TypeName: prop.JsonFieldName,
			JsonName: prop.JsonFieldName,
			Schema:   prop.Schema,
		}
	}
	// Otherwise we cannot flatten the schema, we will purge all properties marked as metadata
	osc := td.Schema.OAPISchema
	for propName := range osc.Properties {
		if stringSliceContains(info.metadataProperties, propName) {
			delete(osc.Properties, propName)
		}
	}
	sc, err := GenerateGoSchema(&openapi3.SchemaRef{Value: osc}, td.Schema.Path)
	if err != nil {
		panic(err)
	}
	return &TypeDefinition{
		TypeName: td.TypeName,
		JsonName: td.JsonName,
		Schema:   sc,
	}
}

// isFlatTypeDefinitionAfterReduction checks if the given ResponseTypeDefinition would be a flat
// type after reduction.
func isFlatTypeDefinitionAfterReduction(td ResponseTypeDefinition, info *primaryResponseInfo) bool {
	return isFlatTypeDefinition(asReducedTypeDefinition(td, info))
}

// genReturnTypeName works similarly to genResponseTypeName, and substitutes the "flat"
// name for the response name if possible.
func genReturnTypeName(op *OperationDefinition) string {
	defaultName := "*" + UppercaseFirstCharacter(genResponseTypeName(op.OperationId))
	td := getPrimaryResponseTypeDefinition(op)
	// No primary response was specified. Use the default.
	if td == nil {
		return defaultName
	}
	info := getPrimaryResponseInfo(op)
	prop := getSingleNonMetadataProperty(&td.TypeDefinition, info)
	// We can't use a flat name. Use the default.
	if prop == nil {
		return defaultName
	}
	return prop.GoTypeDef()
}

// This function map is passed to the template engine, and we can call each
// function here by keyName from the template code.
var TemplateFunctions = template.FuncMap{
	"genParamArgs":               genParamArgs,
	"genParamTypes":              genParamTypes,
	"genParamNames":              genParamNames,
	"genParamFmtString":          ReplacePathParamsWithStr,
	"swaggerUriToEchoUri":        SwaggerUriToEchoUri,
	"swaggerUriToChiUri":         SwaggerUriToChiUri,
	"swaggerUriToGinUri":         SwaggerUriToGinUri,
	"swaggerUriToGorillaUri":     SwaggerUriToGorillaUri,
	"lcFirst":                    LowercaseFirstCharacter,
	"ucFirst":                    UppercaseFirstCharacter,
	"camelCase":                  ToCamelCase,
	"genResponsePayload":         genResponsePayload,
	"genResponseTypeName":        genResponseTypeName,
	"genResponseUnmarshal":       genResponseUnmarshal,
	"getResponseTypeDefinitions": getResponseTypeDefinitions,
	"toStringArray":              toStringArray,
	"lower":                      strings.ToLower,
	"title":                      strings.Title,
	"stripNewLines":              stripNewLines,
	"sanitizeGoIdentity":         SanitizeGoIdentity,

	"getPrimaryResponseInfo":             getPrimaryResponseInfo,
	"getPrimaryResponseTypeDefinition":   getPrimaryResponseTypeDefinition,
	"getSingleNonMetadataProperty":       getSingleNonMetadataProperty,
	"isFlatTypeDefinition":               isFlatTypeDefinition,
	"asReducedTypeDefinition":            asReducedTypeDefinition,
	"isFlatTypeDefinitionAfterReduction": isFlatTypeDefinitionAfterReduction,
	"genReturnTypeName":                  genReturnTypeName,
}
