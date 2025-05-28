package handlersTest

import (
	"testing"

	validator "github.com/pb33f/libopenapi-validator"
)

// TestOpenAPISchemaValidation validates that our openapi.yaml document
// is valid according to the OpenAPI 3.1.1 specification
func TestOpenAPISchemaValidation(t *testing.T) {
	t.Run("Validate OpenAPI Document Against OpenAPI 3.1.1 Specification", func(t *testing.T) {
		// 1. Load the OpenAPI document using the shared helper
		document := LoadOpenAPIDocument(t)

		// 2. Verify the document version is OpenAPI 3.1.x
		if document.GetVersion() != "3.1.1" {
			t.Logf("Warning: Expected OpenAPI version 3.1.1, but got %s", document.GetVersion())
		}

		// 3. Build the document model to catch additional structural issues
		_, modelErrs := document.BuildV3Model()
		if len(modelErrs) > 0 {
			t.Fatalf("Failed to build OpenAPI v3 model: %v", modelErrs)
		}

		// 4. Create a high-level validator for comprehensive document validation
		docValidator, validatorErrs := validator.NewValidator(document)
		if len(validatorErrs) > 0 {
			t.Fatalf("Failed to create document validator: %v", validatorErrs)
		}

		// 5. Perform comprehensive document validation against OpenAPI 3.1.1 spec
		isValid, validationErrors := docValidator.ValidateDocument()

		// 6. Report validation results
		if !isValid {
			t.Errorf("OpenAPI document validation failed with %d error(s):", len(validationErrors))
			for i, validationError := range validationErrors {
				t.Errorf("  Validation Error %d:", i+1)
				t.Errorf("    Type: %s", validationError.ValidationType)
				t.Errorf("    Message: %s", validationError.Message)
				t.Errorf("    Reason: %s", validationError.Reason)
				if validationError.HowToFix != "" {
					t.Errorf("    How to Fix: %s", validationError.HowToFix)
				}
				if validationError.SpecLine > 0 {
					t.Errorf("    Location: Line %d, Column %d", validationError.SpecLine, validationError.SpecCol)
				}

				// Report schema-specific validation errors if present
				if len(validationError.SchemaValidationErrors) > 0 {
					t.Errorf("    Schema Validation Errors:")
					for j, schemaError := range validationError.SchemaValidationErrors {
						t.Errorf("      Schema Error %d:", j+1)
						t.Errorf("        Reason: %s", schemaError.Reason)
						t.Errorf("        Location: %s", schemaError.Location)
						if schemaError.Line > 0 {
							t.Errorf("        Line: %d, Column: %d", schemaError.Line, schemaError.Column)
						}
					}
				}
				t.Errorf("") // Add blank line between errors for readability
			}
		} else {
			t.Logf("✅ OpenAPI document successfully validated against OpenAPI 3.1.1 specification")
		}
	})

	t.Run("Validate External Reference Resolution", func(t *testing.T) {
		// This sub-test specifically checks if external references can be resolved
		// which is important for our schema that references rvinfo.yaml

		// 1. Load the OpenAPI document using the shared helper
		document := LoadOpenAPIDocument(t)

		// 2. Try to build the model which will attempt to resolve all references
		model, modelErrs := document.BuildV3Model()
		if len(modelErrs) > 0 {
			t.Logf("Model building encountered %d error(s):", len(modelErrs))
			for i, err := range modelErrs {
				t.Logf("  Error %d: %v", i+1, err)
			}

			// Check if errors are specifically about external reference resolution
			hasExternalRefErrors := false
			for _, err := range modelErrs {
				if err.Error() != "" {
					hasExternalRefErrors = true
					break
				}
			}

			if hasExternalRefErrors {
				t.Errorf("External reference resolution failed - this may affect API validation")
			}
		} else {
			t.Logf("✅ All external references resolved successfully")
		}

		// 3. Verify we can access the built model's components
		if model != nil && model.Model.Components != nil {
			schemasCount := 0
			if model.Model.Components.Schemas != nil {
				schemasCount = model.Model.Components.Schemas.Len()
			}
			t.Logf("📊 Document contains %d schema components", schemasCount)
		}
	})
}
