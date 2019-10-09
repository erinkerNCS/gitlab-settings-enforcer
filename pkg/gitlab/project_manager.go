package gitlab

import (
  "fmt"
  "net/http"
  "net/url"
  "regexp"
  "strings"

  "github.com/xanzy/go-gitlab"

  "github.com/erinkerNCS/gitlab-settings-enforcer/pkg/config"
  "github.com/erinkerNCS/gitlab-settings-enforcer/pkg/internal/stringslice"

  "github.com/sirupsen/logrus"
)

// ProjectManager fetches a list of repositories from GitLab
type ProjectManager struct {
  logger                  *logrus.Entry
  groupsClient            groupsClient
  projectsClient          projectsClient
  protectedBranchesClient protectedBranchesClient
  branchesClient          branchesClient
  config                  *config.Config
}

// NewProjectManager returns a new ProjectManager instance
func NewProjectManager(
  logger *logrus.Entry,
  groupsClient groupsClient,
  projectsClient projectsClient,
  protectedBranchesClient protectedBranchesClient,
  branchesClient branchesClient,
  config *config.Config,
) *ProjectManager {
  return &ProjectManager{
    logger:                  logger,
    groupsClient:            groupsClient,
    projectsClient:          projectsClient,
    protectedBranchesClient: protectedBranchesClient,
    branchesClient:          branchesClient,
    config:                  config,
  }
}

// GetProjects fetches a list of accessible repos within the groups set in config file
func (m *ProjectManager) GetProjects() ([]Project, error) {
  var repos []Project

  m.logger.Debugf("Fetching projects under %s path ...", m.config.GroupName)

  // Identify Group/Subgroup's ID
  var groupID int
  if strings.ContainsAny(m.config.GroupName, "/") {
    // Nested Path
    m.logger.Debugf("Identifying subgroup's GroupID for %s ...", m.config.GroupName)
    path_tokens := strings.Split(m.config.GroupName, "/")
    var base_group string = path_tokens[0]

    subgroups, _, err := m.groupsClient.ListSubgroups(base_group, listSubgroupOps)
    if err != nil {
      return []Project{}, fmt.Errorf("failed to fetch GitLab subgroups for %q: %v", base_group, err)
    }

    var count int = 1
    for _, g := range subgroups {
      m.logger.Debugf("---[ SubGroup #%d ]---\n", count)
      m.logger.Debugf("%+v\n", g)
      matched, _ := regexp.MatchString("^"+m.config.GroupName+"$", g.FullPath)
      if matched {
        groupID = g.ID
      }
      count += 1
    }
  } else {
    // Only Base
    m.logger.Debugf("Identifying %s's GroupID", m.config.GroupName)
    // BugFix: Without this pre-processing, go-gitlab library stalls.
    var group_name string = strings.Replace(url.PathEscape(m.config.GroupName), ".", "%2E", -1)
    group, _, err := m.groupsClient.GetGroup(group_name)
    if err != nil {
      return []Project{}, fmt.Errorf("failed to fetch GitLab group info for %q: %v", group_name, err)
    }
    groupID = group.ID
  }
  m.logger.Debugf("GroupID is %d", groupID)

  // Get Project objects
  for {
    projects, resp, err := m.groupsClient.ListGroupProjects(groupID, listGroupProjectOps, addIncludeSubgroups)
    if err != nil {
      return []Project{}, fmt.Errorf("failed to fetch GitLab projects for %s [%d]: %v", m.config.GroupName, groupID, err)
    }

    for _, p := range projects {
      if len(m.config.ProjectWhitelist) > 0 && !stringslice.Contains(p.PathWithNamespace, m.config.ProjectWhitelist) {
        m.logger.Debugf("Skipping repo %s as it's not whitelisted", p.PathWithNamespace)
        continue
      }
      if stringslice.Contains(p.PathWithNamespace, m.config.ProjectBlacklist) {
        m.logger.Debugf("Skipping repo %s as it's blacklisted", p.PathWithNamespace)
        continue
      }

      repos = append(repos, Project{
        ID:       p.ID,
        Name:     p.Name,
        FullPath: p.PathWithNamespace,
      })
    }

    // Exit the loop when we've seen all pages.
    if listGroupProjectOps.Page >= resp.TotalPages || resp.TotalPages == 1 {
      break
    }

    // Update the page number to get the next page.
    listGroupProjectOps.Page = resp.NextPage
  }

  m.logger.Debugf("Fetching projects under path done. Retrieved %d.", len(repos))

  return repos, nil
}

// EnsureBranchesAndProtection ensures that 1) the default branch exists and 2) all of the protected branches are configured correctly
func (m *ProjectManager) EnsureBranchesAndProtection(project Project) error {
  if err := m.ensureDefaultBranch(project); err != nil {
    return err
  }

  for _, b := range m.config.ProtectedBranches {
    if resp, err := m.protectedBranchesClient.UnprotectRepositoryBranches(project.ID, b.Name); err != nil && resp.StatusCode != http.StatusNotFound {
      return fmt.Errorf("failed to unprotect branch %v before protection: %v", b.Name, err)
    }

    opt := &gitlab.ProtectRepositoryBranchesOptions{
      Name:             gitlab.String(b.Name),
      PushAccessLevel:  b.PushAccessLevel.Value(),
      MergeAccessLevel: b.MergeAccessLevel.Value(),
    }

    if _, _, err := m.protectedBranchesClient.ProtectRepositoryBranches(project.ID, opt); err != nil {
      return fmt.Errorf("failed to protect branch %s: %v", b.Name, err)
    }
  }

  return nil
}

func (m *ProjectManager) ensureDefaultBranch(project Project) error {
  if !m.config.CreateDefaultBranch ||
    m.config.ProjectSettings.DefaultBranch == nil ||
    *m.config.ProjectSettings.DefaultBranch == "master" {
    return nil
  }

  opt := &gitlab.CreateBranchOptions{
    Branch: m.config.ProjectSettings.DefaultBranch,
    Ref:    gitlab.String("master"),
  }

  m.logger.Debugf("Ensuring default branch %s existence ... ", *opt.Branch)

  _, resp, err := m.branchesClient.GetBranch(project.ID, *opt.Branch)
  if err == nil {
    m.logger.Debugf("Ensuring default branch %s existence ... already exists!", *opt.Branch)
    return nil
  }

  if resp.StatusCode != http.StatusNotFound {
    return fmt.Errorf("failed to check for default branch existence, got unexpected response status code %d", resp.StatusCode)
  }

  if _, _, err := m.branchesClient.CreateBranch(project.ID, opt); err != nil {
    return fmt.Errorf("failed to create default branch %s: %v", *opt.Branch, err)
  }

  return nil
}

// UpdateProjectSettings updates the project settings on gitlab
func (m *ProjectManager) UpdateProjectSettings(project Project) error {
  m.logger.Debugf("Updating settings of project %s ...", project.FullPath)

  m.logger.Debugf("---[ HTTP Payload ]---\n")
  m.logger.Debugf("%+v\n", m.config.ProjectSettings)

  returned_project, response, err := m.projectsClient.EditProject(project.ID, m.config.ProjectSettings)

  m.logger.Debugf("---[ HTTP Response ]---\n")
  m.logger.Debugf("%s\n", response)
  m.logger.Debugf("---[ Returned Project ]---\n")
  m.logger.Debugf("%s\n", returned_project)

  if err != nil {
    return fmt.Errorf("failed to update settings or project %s: %v", project.FullPath, err)
  }

  m.logger.Debugf("Updating settings of project %s done.", project.FullPath)

  return nil
}

// UpdateProjectMergeRequestSettings updates the project settings on gitlab
func (m *ProjectManager) UpdateProjectApprovalSettings(project Project) error {
  m.logger.Debugf("Updating merge request approval settings of project %s [%d]...", project.FullPath, project.ID)

  m.logger.Debugf("---[ HTTP Payload ]---\n")
  m.logger.Debugf("%+v\n", m.config.ApprovalSettings)

  returned_mr, response, err := m.projectsClient.ChangeApprovalConfiguration(project.ID, m.config.ApprovalSettings)

  m.logger.Debugf("---[ HTTP Response ]---\n")
  m.logger.Debugf("%s\n", response)
  m.logger.Debugf("---[ Returned MR ]---\n")
  m.logger.Debugf("%s\n", returned_mr)

  if err != nil {
    return fmt.Errorf("failed to update merge request approval settings or project %s: %v", project.FullPath, err)
  }

  m.logger.Debugf("Updating merge request approval settings of project %s done.", project.FullPath)

  return nil
}
