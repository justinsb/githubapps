package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true

type PullRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PullRequestSpec `json:"spec,omitempty"`
}

type PullRequestSpec struct {
	Title     string      `json:"title,omitempty"`
	URL       string      `json:"url,omitempty"`
	State     string      `json:"state,omitempty"`
	Author    string      `json:"author,omitempty"`
	Body      string      `json:"body,omitempty"`
	CreatedAt metav1.Time `json:"createdAt,omitempty"`
	UpdatedAt metav1.Time `json:"updatedAt,omitempty"`

	Base        *BaseRef     `json:"base,omitempty"`
	Comments    []Comment    `json:"comments,omitempty"`
	CheckSuites []CheckSuite `json:"checkSuites,omitempty"`
	Labels      []Label      `json:"labels,omitempty"`
	Assigned    []Assigned   `json:"assigned,omitempty"`
}

type BaseRef struct {
	Ref string `json:"ref,omitempty"`
	SHA string `json:"sha,omitempty"`
}

type CheckSuite struct {
	ID     int64   `json:"id,omitempty"`
	Checks []Check `json:"checks,omitempty"`
}

type Comment struct {
	Author    string      `json:"author,omitempty"`
	Body      string      `json:"body,omitempty"`
	CreatedAt metav1.Time `json:"createdAt,omitempty"`
}

type Label struct {
	Name      string       `json:"name,omitempty"`
	CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

type Assigned struct {
	Name string `json:"name,omitempty"`
	// CreatedAt *metav1.Time `json:"createdAt,omitempty"`
}

type Check struct {
	Name        string       `json:"name,omitempty"`
	ID          int64        `json:"id,omitempty"`
	Status      string       `json:"status,omitempty"`
	Conclusion  string       `json:"conclusion,omitempty"`
	CompletedAt *metav1.Time `json:"completedAt,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// PullRequestList is a list of PullRequest objects.
type PullRequestList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []PullRequest `json:"items"`
}
