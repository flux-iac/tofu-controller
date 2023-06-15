package provider

import "fmt"

type Repository struct {
	Project string
	Org     string
	Name    string
}

func (r Repository) String() string {
	if r.Project == "" {
		return fmt.Sprintf("%s/%s", r.Org, r.Name)
	}

	return fmt.Sprintf("%s/%s/%s", r.Project, r.Org, r.Name)
}
