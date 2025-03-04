package secrets

import (
	"context"

	"github.com/woodpecker-ci/woodpecker/server/model"
)

type builtin struct {
	context.Context
	store model.SecretStore
}

// New returns a new local secret service.
func New(ctx context.Context, store model.SecretStore) model.SecretService {
	return &builtin{store: store, Context: ctx}
}

func (b *builtin) SecretFind(repo *model.Repo, name string) (*model.Secret, error) {
	return b.store.SecretFind(repo, name)
}

func (b *builtin) SecretList(repo *model.Repo) ([]*model.Secret, error) {
	return b.store.SecretList(repo, false)
}

func (b *builtin) SecretListBuild(repo *model.Repo, build *model.Build) ([]*model.Secret, error) {
	s, err := b.store.SecretList(repo, true)
	if err != nil {
		return nil, err
	}

	// Return only secrets with unique name
	// Priority order in case of duplicate names are repository, user/organization, global
	secrets := make([]*model.Secret, 0, len(s))
	uniq := make(map[string]struct{})
	for _, cond := range []struct {
		Global       bool
		Organization bool
	}{
		{},
		{Organization: true},
		{Global: true},
	} {
		for _, secret := range s {
			if secret.Global() == cond.Global && secret.Organization() == cond.Organization {
				continue
			}
			if _, ok := uniq[secret.Name]; ok {
				continue
			}
			uniq[secret.Name] = struct{}{}
			secrets = append(secrets, secret)
		}
	}
	return secrets, nil
}

func (b *builtin) SecretCreate(repo *model.Repo, in *model.Secret) error {
	return b.store.SecretCreate(in)
}

func (b *builtin) SecretUpdate(repo *model.Repo, in *model.Secret) error {
	return b.store.SecretUpdate(in)
}

func (b *builtin) SecretDelete(repo *model.Repo, name string) error {
	secret, err := b.store.SecretFind(repo, name)
	if err != nil {
		return err
	}
	return b.store.SecretDelete(secret)
}

func (b *builtin) OrgSecretFind(owner, name string) (*model.Secret, error) {
	return b.store.OrgSecretFind(owner, name)
}

func (b *builtin) OrgSecretList(owner string) ([]*model.Secret, error) {
	return b.store.OrgSecretList(owner)
}

func (b *builtin) OrgSecretCreate(owner string, in *model.Secret) error {
	return b.store.SecretCreate(in)
}

func (b *builtin) OrgSecretUpdate(owner string, in *model.Secret) error {
	return b.store.SecretUpdate(in)
}

func (b *builtin) OrgSecretDelete(owner, name string) error {
	secret, err := b.store.OrgSecretFind(owner, name)
	if err != nil {
		return err
	}
	return b.store.SecretDelete(secret)
}

func (b *builtin) GlobalSecretFind(owner string) (*model.Secret, error) {
	return b.store.GlobalSecretFind(owner)
}

func (b *builtin) GlobalSecretList() ([]*model.Secret, error) {
	return b.store.GlobalSecretList()
}

func (b *builtin) GlobalSecretCreate(in *model.Secret) error {
	return b.store.SecretCreate(in)
}

func (b *builtin) GlobalSecretUpdate(in *model.Secret) error {
	return b.store.SecretUpdate(in)
}

func (b *builtin) GlobalSecretDelete(name string) error {
	secret, err := b.store.GlobalSecretFind(name)
	if err != nil {
		return err
	}
	return b.store.SecretDelete(secret)
}
