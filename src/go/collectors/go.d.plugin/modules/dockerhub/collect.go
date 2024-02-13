// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

import (
	"fmt"
	"time"
)

func (dh *DockerHub) collect() (map[string]int64, error) {
	var (
		reposNum = len(dh.Repositories)
		ch       = make(chan *repository, reposNum)
		mx       = make(map[string]int64)
	)

	for _, name := range dh.Repositories {
		go dh.collectRepo(name, ch)
	}

	var (
		parsed  int
		pullSum int
	)

	for i := 0; i < reposNum; i++ {
		repo := <-ch
		if repo == nil {
			continue
		}
		if err := parseRepoTo(repo, mx); err != nil {
			dh.Errorf("error on parsing %s/%s : %v", repo.User, repo.Name, err)
			continue
		}
		pullSum += repo.PullCount
		parsed++
	}
	close(ch)

	if parsed == reposNum {
		mx["pull_sum"] = int64(pullSum)
	}

	return mx, nil
}

func (dh *DockerHub) collectRepo(repoName string, ch chan *repository) {
	repo, err := dh.client.getRepository(repoName)
	if err != nil {
		dh.Error(err)
	}
	ch <- repo
}

func parseRepoTo(repo *repository, mx map[string]int64) error {
	t, err := time.Parse(time.RFC3339Nano, repo.LastUpdated)
	if err != nil {
		return err
	}
	mx[fmt.Sprintf("last_updated_%s/%s", repo.User, repo.Name)] = int64(time.Since(t).Seconds())
	mx[fmt.Sprintf("star_count_%s/%s", repo.User, repo.Name)] = int64(repo.StarCount)
	mx[fmt.Sprintf("pull_count_%s/%s", repo.User, repo.Name)] = int64(repo.PullCount)
	mx[fmt.Sprintf("status_%s/%s", repo.User, repo.Name)] = int64(repo.Status)
	return nil
}
