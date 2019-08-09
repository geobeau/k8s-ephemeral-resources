package main

import (
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func createWatcherContext(name string) WatcherContext {
	return makeWatcherContext(name, time.Minute, func(s string, s2 string, status v1.PodStatus) {
		return
	})
}

func createPodStatus(phase v1.PodPhase, conditions []v1.PodCondition) v1.PodStatus {
	return v1.PodStatus{ phase,
		conditions,
		"",
		"",
		"",
		"",
		"",
		nil,
		nil,
		nil,
		"",
	}

}

func createPodCondition(podType v1.PodConditionType, status v1.ConditionStatus) v1.PodCondition {
	return v1.PodCondition{
		podType,
		status,
		metav1.Time{},
		metav1.Time{},
		"",
		"",
	}
}

func TestClusterState(t *testing.T) {
	assert := assert.New(t)

	watcher := createWatcherContext("ClusterState")
	clusterState := watcher.evaluateClusterState()

	assert.Equal(clusterState.health, HEALTHY, "Empty cluster should be healthy")
	assert.Empty(clusterState.unhealthyPods, "Healthy cluster should not have unhealthy pods")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodPending, nil)
	clusterState = watcher.evaluateClusterState()
	assert.Equal(clusterState.health, UNHEALTHY, "cluster should be unhealthy with a Non running pod")
	assert.NotEmpty(clusterState.unhealthyPods,  "Cluster should have 1 unhealthy")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, nil)
	clusterState = watcher.evaluateClusterState()
	assert.Equal(clusterState.health, HEALTHY, "cluster should be healthy with a Non running pod")
	assert.Empty(clusterState.unhealthyPods, "Healthy cluster should not have unhealthy pods")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, nil)
	watcher.podsStatus["toto2"] = createPodStatus(v1.PodRunning, []v1.PodCondition{createPodCondition(v1.PodScheduled, v1.ConditionTrue)})
	clusterState = watcher.evaluateClusterState()
	assert.Equal(clusterState.health, HEALTHY, "cluster should be healthy with a Non running pod")
	assert.Empty(clusterState.unhealthyPods, "Healthy cluster should not have unhealthy pods")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, nil)
	watcher.podsStatus["toto2"] = createPodStatus(v1.PodRunning, []v1.PodCondition{createPodCondition(v1.PodScheduled, v1.ConditionFalse)})
	clusterState = watcher.evaluateClusterState()
	assert.Equal(clusterState.health, UNHEALTHY, "cluster should be healthy with a Non running pod")
	assert.NotEmpty(clusterState.unhealthyPods,  "Cluster should have 1 unhealthy")
	assert.Equal(len(clusterState.unhealthyPods), 1,  "Cluster should have 1 unhealthy")
}


func TestUpdateClusterState(t *testing.T) {
	assert := assert.New(t)

	watcher := createWatcherContext("UpdateClusterState")
	watcher.updateClusterState()
	currentTime := watcher.clusterState.since

	watcher.updateClusterState()
	assert.Equal(currentTime, watcher.clusterState.since, "Cluster state should not have been updated")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, nil)
	watcher.updateClusterState()
	assert.Equal(currentTime, watcher.clusterState.since, "Cluster state should not have been updated if healthy")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, []v1.PodCondition{createPodCondition(v1.PodScheduled, v1.ConditionFalse)})
	watcher.updateClusterState()
	assert.NotEqual(currentTime, watcher.clusterState.since, "Cluster state should have changed")

	currentTime = watcher.clusterState.since
	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, []v1.PodCondition{createPodCondition(v1.PodScheduled, v1.ConditionFalse)})
	watcher.podsStatus["toto2"] = createPodStatus(v1.PodRunning, nil)
	watcher.updateClusterState()
	assert.Equal(currentTime, watcher.clusterState.since, "Cluster state should not change")

	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, []v1.PodCondition{createPodCondition(v1.PodScheduled, v1.ConditionFalse)})
	watcher.podsStatus["toto2"] = createPodStatus(v1.PodPending, nil)
	watcher.updateClusterState()
	assert.NotEqual(currentTime, watcher.clusterState.since, "Cluster state should have change")
}

func TestRetaliate(t *testing.T) {
	assert := assert.New(t)
	watcher := createWatcherContext("Retaliate")
	ret := retaliate(&watcher)

	assert.False(ret, "HEALTHY cluster should not trigger retaliation")


	watcher.podsStatus["toto1"] = createPodStatus(v1.PodRunning, nil)
	watcher.updateClusterState()
	ret = retaliate(&watcher)
	assert.False(ret, "HEALTHY cluster should not trigger retaliation")

	watcher.podsStatus["toto2"] = createPodStatus(v1.PodPending, nil)
	watcher.updateClusterState()
	ret = retaliate(&watcher)
	assert.False(ret, "UNHEALTHY cluster still in the grace period should not trigger retaliation")

	watcher.clusterState.since = time.Now().Add(-2 * time.Minute)
	ret = retaliate(&watcher)
	assert.True(ret, "UNHEALTHY cluster still where the grace period elapsed shoud trigger retaliation")

	watcher.podsStatus["toto3"] = createPodStatus(v1.PodPending, nil)
	watcher.updateClusterState()
	ret = retaliate(&watcher)
	assert.False(ret, "UNHEALTHY cluster with more than 1 unhealthy pods should do nothing")

	delete(watcher.podsStatus, "toto2")
	watcher.updateClusterState()
	ret = retaliate(&watcher)
	assert.False(ret, "changing cluster state should update the since time and not trigger retaliation")

	watcher.clusterState.since = time.Now().Add(-2 * time.Minute)
	ret = retaliate(&watcher)
	assert.True(ret, "UNHEALTHY cluster still where the grace period elapsed shoud trigger retaliation")
}
