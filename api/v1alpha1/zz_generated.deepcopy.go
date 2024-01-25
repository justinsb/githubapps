//go:build !ignore_autogenerated
// +build !ignore_autogenerated

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *BaseRef) DeepCopyInto(out *BaseRef) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new BaseRef.
func (in *BaseRef) DeepCopy() *BaseRef {
	if in == nil {
		return nil
	}
	out := new(BaseRef)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Check) DeepCopyInto(out *Check) {
	*out = *in
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Check.
func (in *Check) DeepCopy() *Check {
	if in == nil {
		return nil
	}
	out := new(Check)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CheckSuite) DeepCopyInto(out *CheckSuite) {
	*out = *in
	if in.Checks != nil {
		in, out := &in.Checks, &out.Checks
		*out = make([]Check, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CheckSuite.
func (in *CheckSuite) DeepCopy() *CheckSuite {
	if in == nil {
		return nil
	}
	out := new(CheckSuite)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Comment) DeepCopyInto(out *Comment) {
	*out = *in
	in.CreatedAt.DeepCopyInto(&out.CreatedAt)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Comment.
func (in *Comment) DeepCopy() *Comment {
	if in == nil {
		return nil
	}
	out := new(Comment)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Label) DeepCopyInto(out *Label) {
	*out = *in
	if in.CreatedAt != nil {
		in, out := &in.CreatedAt, &out.CreatedAt
		*out = (*in).DeepCopy()
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Label.
func (in *Label) DeepCopy() *Label {
	if in == nil {
		return nil
	}
	out := new(Label)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PullRequest) DeepCopyInto(out *PullRequest) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PullRequest.
func (in *PullRequest) DeepCopy() *PullRequest {
	if in == nil {
		return nil
	}
	out := new(PullRequest)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PullRequest) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PullRequestList) DeepCopyInto(out *PullRequestList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]PullRequest, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PullRequestList.
func (in *PullRequestList) DeepCopy() *PullRequestList {
	if in == nil {
		return nil
	}
	out := new(PullRequestList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *PullRequestList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *PullRequestSpec) DeepCopyInto(out *PullRequestSpec) {
	*out = *in
	in.CreatedAt.DeepCopyInto(&out.CreatedAt)
	in.UpdatedAt.DeepCopyInto(&out.UpdatedAt)
	if in.Base != nil {
		in, out := &in.Base, &out.Base
		*out = new(BaseRef)
		**out = **in
	}
	if in.Comments != nil {
		in, out := &in.Comments, &out.Comments
		*out = make([]Comment, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.CheckSuites != nil {
		in, out := &in.CheckSuites, &out.CheckSuites
		*out = make([]CheckSuite, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.Labels != nil {
		in, out := &in.Labels, &out.Labels
		*out = make([]Label, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new PullRequestSpec.
func (in *PullRequestSpec) DeepCopy() *PullRequestSpec {
	if in == nil {
		return nil
	}
	out := new(PullRequestSpec)
	in.DeepCopyInto(out)
	return out
}