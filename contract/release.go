package contract

import (
	_ "embed"
	"encoding/json"
)

//go:embed dependency-lock.json
var dependencyLock []byte

//go:embed generated/policy/release-admission.v1.json
var admissionPolicy []byte

type ReleaseIdentity struct {
	Version string `json:"version"`
	Archive struct {
		SHA256 string `json:"sha256"`
	} `json:"archive"`
	Executable struct {
		Target string `json:"target"`
		SHA256 string `json:"sha256"`
	} `json:"executable"`
}

type OperationAdmission struct {
	Method           string   `json:"method"`
	RequiredFeatures []string `json:"required_features"`
	ReleaseAdmitted  bool     `json:"release_admitted"`
}

type ReleaseAdmission struct {
	CredentialSchemaID string               `json:"credential_schema_id"`
	Operations         []OperationAdmission `json:"operations"`
}

// QualifiedRelease returns the checked release authority embedded in this build.
func QualifiedRelease() ReleaseIdentity {
	var lock struct {
		Release ReleaseIdentity `json:"release"`
	}
	if err := json.Unmarshal(dependencyLock, &lock); err != nil {
		panic(err)
	}
	return lock.Release
}

func Admission() ReleaseAdmission {
	var policy ReleaseAdmission
	if err := json.Unmarshal(admissionPolicy, &policy); err != nil {
		panic(err)
	}
	return policy
}
