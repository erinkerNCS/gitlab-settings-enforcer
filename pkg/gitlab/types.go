package gitlab

import (
  "net/http"

  gitlab "github.com/xanzy/go-gitlab"
)

// currentState stores current Project state for each project interacted with
type ProjectSettings struct {
  Approval gitlab.ProjectApprovals `json:"approval_settings,omitempty"`
  General  gitlab.Project          `json:"project_settings,omitempty"`
}

type groupsClient interface {
  GetGroup(gid interface{}, options ...gitlab.OptionFunc) (*gitlab.Group, *gitlab.Response, error)
  ListGroupProjects(gid interface{}, opt *gitlab.ListGroupProjectsOptions, options ...gitlab.OptionFunc) ([]*gitlab.Project, *gitlab.Response, error)
  ListSubgroups(gid interface{}, opt *gitlab.ListSubgroupsOptions, options ...gitlab.OptionFunc) ([]*gitlab.Group, *gitlab.Response, error)
}

type projectsClient interface {
  ChangeApprovalConfiguration(pid interface{}, opt *gitlab.ChangeApprovalConfigurationOptions, options ...gitlab.OptionFunc) (*gitlab.ProjectApprovals, *gitlab.Response, error)
  CreateProjectApprovalRule(pid interface{}, opt *gitlab.CreateProjectLevelRuleOptions, options ...gitlab.OptionFunc) (*gitlab.ProjectApprovalRule, *gitlab.Response, error)
  DeleteProjectApprovalRule(pid interface{}, approvalRule int, options ...gitlab.OptionFunc) (*gitlab.Response, error)
  GetApprovalConfiguration(pid interface{}, options ...gitlab.OptionFunc) (*gitlab.ProjectApprovals, *gitlab.Response, error)
  GetProject(pid interface{}, opt *gitlab.GetProjectOptions, options ...gitlab.OptionFunc) (*gitlab.Project, *gitlab.Response, error)
  GetProjectApprovalRules(pid interface{}, options ...gitlab.OptionFunc) ([]*gitlab.ProjectApprovalRule, *gitlab.Response, error)
  EditProject(pid interface{}, opt *gitlab.EditProjectOptions, options ...gitlab.OptionFunc) (*gitlab.Project, *gitlab.Response, error)
  UpdateProjectApprovalRule(pid interface{}, approvalRule int, opt *gitlab.UpdateProjectLevelRuleOptions, options ...gitlab.OptionFunc) (*gitlab.ProjectApprovalRule, *gitlab.Response, error)
}

type protectedBranchesClient interface {
  ProtectRepositoryBranches(pid interface{}, opt *gitlab.ProtectRepositoryBranchesOptions, options ...gitlab.OptionFunc) (*gitlab.ProtectedBranch, *gitlab.Response, error)
  UnprotectRepositoryBranches(pid interface{}, branch string, options ...gitlab.OptionFunc) (*gitlab.Response, error)
  // ListProtectedBranches(pid interface{}, opt *gitlab.ListProtectedBranchesOptions, options ...gitlab.OptionFunc) ([]*gitlab.ProtectedBranch, *gitlab.Response, error)
}

type branchesClient interface {
  CreateBranch(pid interface{}, opt *gitlab.CreateBranchOptions, options ...gitlab.OptionFunc) (*gitlab.Branch, *gitlab.Response, error)
  GetBranch(pid interface{}, branch string, options ...gitlab.OptionFunc) (*gitlab.Branch, *gitlab.Response, error)
}

var (
  listGroupProjectOps = &gitlab.ListGroupProjectsOptions{
    ListOptions: gitlab.ListOptions{
      PerPage: 100,
    },
  }

  listSubgroupOps = &gitlab.ListSubgroupsOptions{
    ListOptions: gitlab.ListOptions{
      PerPage: 100,
    },
  }

  addIncludeSubgroups = gitlab.OptionFunc(func(req *http.Request) error {
    v := req.URL.Query()
    v.Add("include_subgroups", "true")
    req.URL.RawQuery = v.Encode()
    return nil
  })
)
