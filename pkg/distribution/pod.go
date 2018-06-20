//
// Last.Backend LLC CONFIDENTIAL
// __________________
//
// [2014] - [2018] Last.Backend LLC
// All Rights Reserved.
//
// NOTICE:  All information contained herein is, and remains
// the property of Last.Backend LLC and its suppliers,
// if any.  The intellectual and technical concepts contained
// herein are proprietary to Last.Backend LLC
// and its suppliers and may be covered by Russian Federation and Foreign Patents,
// patents in process, and are protected by trade secret or copyright law.
// Dissemination of this information or reproduction of this material
// is strictly forbidden unless prior written permission is obtained
// from Last.Backend LLC.
//

package distribution

import (
	"context"
	"github.com/lastbackend/lastbackend/pkg/distribution/types"
	"github.com/lastbackend/lastbackend/pkg/log"
	"github.com/lastbackend/lastbackend/pkg/storage"
	"github.com/lastbackend/lastbackend/pkg/storage/etcd/v3/store"
	"github.com/lastbackend/lastbackend/pkg/util/generator"
	"github.com/spf13/viper"
	"strings"
	"time"
	"github.com/lastbackend/lastbackend/pkg/storage/etcd"
)

const (
	logPodPrefix = "distribution:pod"
)

type IPod interface {
	Get(namespace, service, deployment, name string) (*types.Pod, error)
	Create(deployment *types.Deployment) (*types.Pod, error)
	ListByNamespace(namespace string) (map[string]*types.Pod, error)
	ListByService(namespace, service string) (map[string]*types.Pod, error)
	ListByDeployment(namespace, service, deployment string) (map[string]*types.Pod, error)
	SetNode(pod *types.Pod, node *types.Node) error
	SetStatus(pod *types.Pod, state *types.PodStatus) error
	Destroy(ctx context.Context, pod *types.Pod) error
	Remove(ctx context.Context, pod *types.Pod) error
}

type Pod struct {
	context context.Context
	storage storage.Storage
}

// Get pod info from storage
func (p *Pod) Get(namespace, service, deployment, pod string) (*types.Pod, error) {
	log.V(logLevel).Debugf("%s:get:> get by name %s", logPodPrefix, pod)

	item := new(types.Pod)
	name := etcd.BuildPodKey(namespace, service, deployment, pod)

	err := p.storage.Get(p.context, storage.PodKind, name, &item)
	if err != nil {

		if err.Error() == store.ErrEntityNotFound {
			log.V(logLevel).Warnf("%s:get:> `%s` not found", logPodPrefix, name)
			return nil, nil
		}

		log.V(logLevel).Debugf("%s:get:> get Pod `%s` err: %v", logPodPrefix, name, err)
		return nil, err
	}

	return item, nil
}

// Create new pod
func (p *Pod) Create(deployment *types.Deployment) (*types.Pod, error) {

	pod := types.NewPod()
	pod.Meta.SetDefault()
	pod.Meta.Name = strings.Split(generator.GetUUIDV4(), "-")[4][5:]
	pod.Meta.Deployment = deployment.Meta.Name
	pod.Meta.Service = deployment.Meta.Service
	pod.Meta.Namespace = deployment.Meta.Namespace

	pod.Status.SetInitialized()
	pod.Status.Steps = make(map[string]types.PodStep)
	pod.Status.Steps[types.StepInitialized] = types.PodStep{
		Ready:     true,
		Timestamp: time.Now().UTC(),
	}

	var ips = make([]string, 0)
	viper.UnmarshalKey("dns.ips", &ips)
	ips = append(ips, "8.8.8.8")

	for _, s := range deployment.Spec.Template.Containers {
		s.Labels = make(map[string]string)
		s.Labels["LB"] = pod.SelfLink()
		s.DNS = types.SpecTemplateContainerDNS{
			Server: ips,
			Search: ips,
		}
		pod.Spec.Template.Containers = append(pod.Spec.Template.Containers, s)
	}

	for _, s := range deployment.Spec.Template.Volumes {
		pod.Spec.Template.Volumes = append(pod.Spec.Template.Volumes, s)
	}

	if err := p.storage.Create(p.context, storage.PodKind, pod.Meta.SelfLink, pod, nil); err != nil {
		log.Errorf("%s:create:> insert pod err %v", logPodPrefix, err)
		return nil, err
	}

	return pod, nil
}

// ListByNamespace returns pod list in selected namespace
func (p *Pod) ListByNamespace(namespace string) (map[string]*types.Pod, error) {
	log.V(logLevel).Debugf("%s:listbynamespace:> get pod list by namespace %s", logPodPrefix, namespace)

	q := etcd.BuildPodQuery(namespace, types.EmptyString, types.EmptyString)
	items := make(map[string]*types.Pod, 0)

	err := p.storage.Map(p.context, storage.PodKind, q, items)
	if err != nil {
		log.V(logLevel).Debugf("%s:listbynamespace:> get pod list by deployment id `%s` err: %v", logPodPrefix, namespace, err)
		return nil, err
	}

	return items, nil
}

// ListByService returns pod list in selected service
func (p *Pod) ListByService(namespace, service string) (map[string]*types.Pod, error) {
	log.V(logLevel).Debugf("%s:listbyservice:> get pod list by service id %s/%s", logPodPrefix, namespace, service)

	q := etcd.BuildPodQuery(namespace, service, types.EmptyString)
	items := make(map[string]*types.Pod, 0)

	err := p.storage.Map(p.context, storage.PodKind, q, items)
	if err != nil {
		log.V(logLevel).Debugf("%s:listbyservice:> get pod list by service id `%s` err: %v", logPodPrefix, namespace, service, err)
		return nil, err
	}

	return items, nil
}

// ListByDeployment returns pod list in selected deployment
func (p *Pod) ListByDeployment(namespace, service, deployment string) (map[string]*types.Pod, error) {
	log.V(logLevel).Debugf("%s:listbydeployment:> get pod list by id %s/%s/%s", logPodPrefix, namespace, service, deployment)

	q := etcd.BuildPodQuery(namespace, service, deployment)
	items := make(map[string]*types.Pod, 0)

	err := p.storage.Map(p.context, storage.PodKind, q, items)
	if err != nil {
		log.V(logLevel).Debugf("%s:listbydeployment:> get pod list by deployment id `%s/%s/%s` err: %v",
			logPodPrefix, namespace, service, deployment, err)
		return nil, err
	}

	return items, nil
}

func (p *Pod) SetNode(pod *types.Pod, node *types.Node) error {
	log.Debugf("%s:setnode:> set node for pod: %s", logPodPrefix, pod.Meta.Name)

	pod.Meta.Node = node.Meta.Name

	if err := p.storage.Update(p.context, storage.PodKind, pod.Meta.SelfLink, pod, nil); err != nil {
		log.Errorf("%s:setnode:> pod set node err: %v", logPodPrefix, err)
		return err
	}

	return nil
}

// SetStatus - set state for pod
func (p *Pod) SetStatus(pod *types.Pod, status *types.PodStatus) error {

	log.Debugf("%s:setstatus:> set state for pod: %s", logPodPrefix, pod.Meta.Name)

	pod.Status = *status

	if err := p.storage.Update(p.context, storage.PodKind, pod.Meta.SelfLink, pod, nil); err != nil {
		log.Errorf("%s:setstatus:> pod set status err: %v", logPodPrefix, err)
		return err
	}

	return nil
}

// Destroy pod
func (p *Pod) Destroy(ctx context.Context, pod *types.Pod) error {

	pod.Spec.State.Destroy = true

	if err := p.storage.Update(p.context, storage.PodKind, pod.Meta.SelfLink, pod, nil); err != nil {
		log.Errorf("%s:destroy:> mark pod for destroy error: %v", logPodPrefix, err)
		return err
	}
	return nil
}

// Remove pod from storage
func (p *Pod) Remove(ctx context.Context, pod *types.Pod) error {
	if err := p.storage.Remove(p.context, storage.PodKind, pod.Meta.SelfLink); err != nil {
		log.Errorf("%s:remove:> mark pod for destroy error: %v", logPodPrefix, err)
		return err
	}
	return nil
}

func NewPodModel(ctx context.Context, stg storage.Storage) IPod {
	return &Pod{ctx, stg}
}
