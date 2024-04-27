package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

//go:generate go run sigs.k8s.io/controller-tools/cmd/controller-gen@v0.14.0 object crd:crdVersions=v1 output:crd:artifacts:config=config/ paths=./...

//+kubebuilder:object:generate=true
//+groupName=automation.kpt.dev
//+versionName=v1alpha1

var (
	// GroupVersion is group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "automation.kpt.dev", Version: "v1alpha1"}

	// SchemeBuilder is used to add go types to the GroupVersionKind scheme.
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme

	PullRequestGVK = GroupVersion.WithKind("PullRequest")
)

func init() {
	SchemeBuilder.Register(&PullRequest{}, &PullRequestList{})
}
