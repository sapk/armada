package context

import (
	ctx "context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	errors2 "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
	clientTesting "k8s.io/client-go/testing"

	util2 "github.com/G-Research/armada/internal/common/util"
	"github.com/G-Research/armada/internal/executor/configuration"
	"github.com/G-Research/armada/internal/executor/domain"
	"github.com/G-Research/armada/internal/executor/util"
)

func setupTest() (*KubernetesClusterContext, *fake.Clientset) {
	context, provider := setupTestWithMinRepeatedDeletePeriod(2 * time.Minute)
	return context, provider.FakeClient
}

func setupTestWithProvider() (*KubernetesClusterContext, *FakeClientProvider) {
	return setupTestWithMinRepeatedDeletePeriod(2 * time.Minute)
}

func setupTestWithMinRepeatedDeletePeriod(minRepeatedDeletePeriod time.Duration) (*KubernetesClusterContext, *FakeClientProvider) {
	prometheus.DefaultRegisterer = prometheus.NewRegistry()

	client := fake.NewSimpleClientset()
	clientProvider := &FakeClientProvider{FakeClient: client}

	clusterContext := NewClusterContext(
		configuration.ApplicationConfiguration{ClusterId: "test-cluster-1", Pool: "pool"},
		minRepeatedDeletePeriod,
		clientProvider,
	)

	return clusterContext, clientProvider
}

func TestKubernetesClusterContext_SubmitPod(t *testing.T) {
	clusterContext, client := setupTest()

	pod := createBatchPod()
	client.Fake.ClearActions()

	_, err := clusterContext.SubmitPod(pod, "user1")
	assert.Nil(t, err)

	assert.Equal(t, len(client.Fake.Actions()), 1)
	assert.True(t, client.Fake.Actions()[0].Matches("create", "pods"))

	createAction, ok := client.Fake.Actions()[0].(clientTesting.CreateAction)
	assert.True(t, ok)
	assert.Equal(t, createAction.GetObject(), pod)
}

func TestKubernetesClusterContext_ProcessPodsToDelete_DoesNotCallClient_WhenNoPodsMarkedForDeletion(t *testing.T) {
	clusterContext, client := setupTest()

	client.Fake.ClearActions()
	clusterContext.ProcessPodsToDelete()

	assert.Equal(t, len(client.Fake.Actions()), 0)
}

func TestKubernetesClusterContext_ProcessPodsToDelete_CallDeleteOnClient_WhenPodsMarkedForDeletion(t *testing.T) {
	clusterContext, client := setupTest()

	pod := createSubmittedBatchPod(t, clusterContext)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()

	assert.Equal(t, len(client.Fake.Actions()), 1)
	assert.True(t, client.Fake.Actions()[0].Matches("delete", "pods"))

	deleteAction, ok := client.Fake.Actions()[0].(clientTesting.DeleteAction)
	assert.True(t, ok)
	assert.Equal(t, deleteAction.GetName(), pod.Name)
}

func TestKubernetesClusterContext_ProcessPodsToDelete_PreventsRepeatedDeleteCallsToClient_OnClientSuccess(t *testing.T) {
	clusterContext, client := setupTest()

	pod := createSubmittedBatchPod(t, clusterContext)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 1)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 0)
}

func TestKubernetesClusterContext_ProcessPodsToDelete_PreventsRepeatedDeleteCallsToClient_OnClientErrorNotFound(t *testing.T) {
	clusterContext, client := setupTest()
	client.Fake.PrependReactor("delete", "pods", func(action clientTesting.Action) (bool, runtime.Object, error) {
		notFound := errors2.StatusError{
			ErrStatus: metav1.Status{
				Reason: metav1.StatusReasonNotFound,
			},
		}
		return true, nil, &notFound
	})

	pod := createSubmittedBatchPod(t, clusterContext)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 1)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 0)
}

func TestKubernetesClusterContext_ProcessPodsToDelete_AllowsRepeatedDeleteCallToClient_OnClientError(t *testing.T) {
	clusterContext, client := setupTest()
	client.Fake.PrependReactor("delete", "pods", func(action clientTesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("server error")
	})

	pod := createSubmittedBatchPod(t, clusterContext)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 1)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 1)
}

func TestKubernetesClusterContext_ProcessPodsToDelete_AllowsRepeatedDeleteCallToClient_AfterMinimumDeletePeriodHasPassed(t *testing.T) {
	timeBetweenRepeatedDeleteCalls := 500 * time.Millisecond
	clusterContext, provider := setupTestWithMinRepeatedDeletePeriod(timeBetweenRepeatedDeleteCalls)
	client := provider.FakeClient

	pod := createSubmittedBatchPod(t, clusterContext)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 1)

	//Wait time required between repeated delete calls
	time.Sleep(timeBetweenRepeatedDeleteCalls + 200*time.Millisecond)

	client.Fake.ClearActions()
	clusterContext.DeletePods([]*v1.Pod{pod})
	clusterContext.ProcessPodsToDelete()
	assert.Equal(t, len(client.Fake.Actions()), 1)
}

func TestKubernetesClusterContext_AddAnnotation(t *testing.T) {
	clusterContext, _ := setupTest()
	pod := createSubmittedBatchPod(t, clusterContext)

	annotationsToAdd := map[string]string{"test": "annotation"}
	err := clusterContext.AddAnnotation(pod, annotationsToAdd)
	assert.Nil(t, err)

	updateOccurred := waitForCondition(func() bool {
		pods, err := clusterContext.GetActiveBatchPods()
		assert.Nil(t, err)
		return len(pods) > 0 && pods[0].Annotations["test"] == "annotation"
	})
	assert.True(t, updateOccurred)
}

func TestKubernetesClusterContext_AddAnnotation_ReturnsError_OnClientError(t *testing.T) {
	clusterContext, client := setupTest()
	client.Fake.PrependReactor("patch", "pods", func(action clientTesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("server error")
	})

	pod := createSubmittedBatchPod(t, clusterContext)

	annotationsToAdd := map[string]string{"test": "\\"}
	err := clusterContext.AddAnnotation(pod, annotationsToAdd)
	assert.NotNil(t, err)
}

func TestKubernetesClusterContext_GetAllPods(t *testing.T) {
	clusterContext, _ := setupTest()

	nonBatchPod := createPod()
	batchPod := createBatchPod()
	submitPodsWithWait(t, clusterContext, nonBatchPod, batchPod)

	clusterContext.Stop() //This prevents newly submitted pods being picked up by the informers
	transientBatchPod := createBatchPod()
	submitPod(t, clusterContext, transientBatchPod)

	allPods, err := clusterContext.GetAllPods()

	assert.Nil(t, err)
	assert.Equal(t, len(allPods), 3)

	//Confirm the pods returned are the ones created
	podSet := util2.StringListToSet(util.ExtractNames(allPods))
	assert.True(t, podSet[nonBatchPod.Name])
	assert.True(t, podSet[batchPod.Name])
	assert.True(t, podSet[transientBatchPod.Name])
}

func TestKubernetesClusterContext_GetAllPods_DeduplicatesTransientPods(t *testing.T) {
	clusterContext, _ := setupTest()

	batchPod := createSubmittedBatchPod(t, clusterContext)

	//Forcibly add pod back to cache, so now it exists in kubernetes + cache
	clusterContext.submittedPods.Add(batchPod)

	allPods, err := clusterContext.GetAllPods()

	assert.Nil(t, err)
	assert.Equal(t, len(allPods), 1)

	//Confirm the pods returned are the ones created
	podSet := util2.StringListToSet(util.ExtractNames(allPods))
	assert.True(t, podSet[batchPod.Name])
}

func TestKubernetesClusterContext_GetBatchPods_ReturnsOnlyBatchPods_IncludingTransient(t *testing.T) {
	clusterContext, _ := setupTest()

	nonBatchPod := createPod()
	batchPod := createBatchPod()
	submitPodsWithWait(t, clusterContext, nonBatchPod, batchPod)

	clusterContext.Stop() //This prevents newly submitted pods being picked up by the informers
	transientBatchPod := createBatchPod()
	submitPod(t, clusterContext, transientBatchPod)

	allPods, err := clusterContext.GetBatchPods()

	assert.Nil(t, err)
	assert.Equal(t, len(allPods), 2)

	//Confirm the pods returned are the ones created
	podSet := util2.StringListToSet(util.ExtractNames(allPods))
	assert.True(t, podSet[batchPod.Name])
	assert.True(t, podSet[transientBatchPod.Name])
}

func TestKubernetesClusterContext_GetBatchPods_DoesNotShowTransient_OnSubmitFailure(t *testing.T) {
	clusterContext, client := setupTest()
	client.Fake.PrependReactor("create", "pods", func(action clientTesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("server error")
	})

	batchPod := createBatchPod()
	_, err := clusterContext.SubmitPod(batchPod, "user")
	assert.NotNil(t, err)

	allPods, err := clusterContext.GetBatchPods()

	assert.Nil(t, err)
	assert.Equal(t, len(allPods), 0)
}

func TestKubernetesClusterContext_GetBatchPods_DeduplicatesTransientPods(t *testing.T) {
	clusterContext, _ := setupTest()

	batchPod := createSubmittedBatchPod(t, clusterContext)

	//Forcibly add pod back to cache, so now it exists in kubernetes + cache
	clusterContext.submittedPods.Add(batchPod)

	allPods, err := clusterContext.GetBatchPods()

	assert.Nil(t, err)
	assert.Equal(t, len(allPods), 1)

	//Confirm the pods returned are the ones created
	podSet := util2.StringListToSet(util.ExtractNames(allPods))
	assert.True(t, podSet[batchPod.Name])
}

func TestKubernetesClusterContext_GetActiveBatchPods_ReturnsOnlyBatchPods_ExcludingTransient(t *testing.T) {
	clusterContext, _ := setupTest()

	nonBatchPod := createPod()
	batchPod := createBatchPod()
	submitPodsWithWait(t, clusterContext, batchPod, nonBatchPod)

	clusterContext.Stop() //This prevents newly submitted pods being picked up by the informers
	transientBatchPod := createBatchPod()
	submitPod(t, clusterContext, transientBatchPod)

	allPods, err := clusterContext.GetActiveBatchPods()

	assert.Nil(t, err)
	assert.Equal(t, len(allPods), 1)

	podSet := util2.StringListToSet(util.ExtractNames(allPods))
	assert.True(t, podSet[batchPod.Name])
}

func TestKubernetesClusterContext_GetNodes(t *testing.T) {
	clusterContext, client := setupTest()

	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "Node1",
		},
	}

	_, err := client.CoreV1().Nodes().Create(ctx.Background(), node, metav1.CreateOptions{})
	assert.Nil(t, err)

	nodeFound := waitForCondition(func() bool {
		nodes, err := clusterContext.GetNodes()
		assert.Nil(t, err)
		return len(nodes) > 0 && nodes[0].Name == node.Name
	})
	assert.True(t, nodeFound)
}

func TestKubernetesClusterContext_Submit_UseUserSpecificClient(t *testing.T) {
	clusterContext, provider := setupTestWithProvider()

	_, err := clusterContext.SubmitPod(createBatchPod(), "user")
	assert.Nil(t, err)
	assert.Contains(t, provider.users, "user")
}

func submitPod(t *testing.T, context ClusterContext, pod *v1.Pod) {
	_, err := context.SubmitPod(pod, "user")
	assert.Nil(t, err)
}

func submitPodsWithWait(t *testing.T, context *KubernetesClusterContext, pods ...*v1.Pod) {
	for _, pod := range pods {
		submitPod(t, context, pod)
	}

	waitForContextSync(t, context, pods)
}

func waitForContextSync(t *testing.T, context *KubernetesClusterContext, pods []*v1.Pod) {
	synced := waitForCondition(func() bool {
		existingPods, err := context.podInformer.Lister().List(labels.Everything())
		assert.Nil(t, err)

		allSync := true
		existPodNamesMap := util2.StringListToSet(util.ExtractNames(existingPods))
		for _, pod := range pods {
			if _, present := existPodNamesMap[pod.Name]; !present {
				allSync = false
				break
			}
			if cachedPod := context.submittedPods.Get(pod.Labels[domain.JobId]); cachedPod != nil {
				allSync = false
				break
			}
		}
		return allSync
	})
	if !synced {
		t.Error("Timed out waiting for context sync. Submitted pods were never synced to context.")
		t.Fail()
	}
}

func waitForCondition(conditionCheck func() bool) bool {
	conditionMet := false
	for i := 0; i < 20; i++ {
		conditionMet = conditionCheck()
		if conditionMet {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	return conditionMet
}

func createSubmittedBatchPod(t *testing.T, context *KubernetesClusterContext) *v1.Pod {
	pod := createBatchPod()
	submitPodsWithWait(t, context, pod)
	return pod
}

func createBatchPod() *v1.Pod {
	pod := createPod()
	pod.ObjectMeta.Labels = map[string]string{
		domain.JobId: "jobid" + util2.NewULID(),
	}
	return pod
}

func createPod() *v1.Pod {
	return &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      util2.NewULID(),
			Namespace: "default",
		},
	}
}

type FakeClientProvider struct {
	FakeClient *fake.Clientset
	users      []string
}

func (p *FakeClientProvider) ClientForUser(user string) (kubernetes.Interface, error) {
	p.users = append(p.users, user)
	return p.FakeClient, nil
}
func (p *FakeClientProvider) Client() kubernetes.Interface {
	return p.FakeClient
}

func (p *FakeClientProvider) ClientConfig() *rest.Config {
	return nil
}
