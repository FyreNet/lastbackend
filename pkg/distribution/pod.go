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
	"fmt"
	"github.com/lastbackend/lastbackend/pkg/distribution/errors"
	"github.com/lastbackend/lastbackend/pkg/distribution/types"
	"github.com/lastbackend/lastbackend/pkg/log"
	"github.com/lastbackend/lastbackend/pkg/storage"
	"github.com/lastbackend/lastbackend/pkg/util/generator"
	"github.com/spf13/viper"
	"strings"
	"time"
)

type IPod interface {
	Get(namespace, service, deployment, name string) (*types.Pod, error)
	Create(deployment *types.Deployment) (*types.Pod, error)
	ListByNamespace(namespace string) (map[string]*types.Pod, error)
	ListByService(namespace, service string) (map[string]*types.Pod, error)
	ListByDeployment(namespace, service, deployment string) (map[string]*types.Pod, error)
	Schedule(pod *types.Pod) (*types.Pod, error)
	SetState(pod *types.Pod) (*types.Pod, error)
	Destroy(ctx context.Context, pod *types.Pod) error
	Remove(ctx context.Context, pod *types.Pod) error
}

type Pod struct {
	context context.Context
	storage storage.Storage
}

// Get pod info from storage
func (p *Pod) Get(namespace, service, deployment, name string) (*types.Pod, error) {
	log.V(logLevel).Debugf("Pod: get by name %s", name)

	pod, err := p.storage.Pod().Get(p.context, namespace, service, deployment, name)
	if err != nil {
		log.V(logLevel).Debugf("Pod: get Pod `%s` err: %s", name, err)
		return nil, err
	}

	return pod, nil
}

// Create new pod
func (p *Pod) Create(deployment *types.Deployment) (*types.Pod, error) {

	pod := types.NewPod()
	pod.Meta.SetDefault()
	pod.Meta.Name = strings.Split(generator.GetUUIDV4(), "-")[4][5:]
	pod.Meta.SelfLink = fmt.Sprintf("%s/pod/%s", deployment.Meta.SelfLink, pod.Meta.Name)
	pod.Meta.Deployment = deployment.Meta.Name
	pod.Meta.Service = deployment.Meta.Service
	pod.Meta.Namespace = deployment.Meta.Namespace

	pod.MarkAsInitialized()
	pod.Status.Steps = make(map[string]types.PodStep)
	pod.Status.Steps[types.PodStepInitialized] = types.PodStep{
		Ready:     true,
		Timestamp: time.Now().UTC(),
	}

	var ips = make([]string, 0)
	viper.UnmarshalKey("dns.ips", &ips)
	ips = append(ips, "8.8.8.8")

	for _, s := range deployment.Spec.Template.Containers {
		s.Labels = make(map[string]string)
		s.Labels["LB"] = fmt.Sprintf("%s/%s/%s", pod.Meta.Namespace, pod.Meta.Deployment, pod.Meta.Name)
		s.DNS = types.SpecTemplateContainerDNS{
			Server: ips,
			Search: ips,
		}
		pod.Spec.Template.Containers = append(pod.Spec.Template.Containers, s)
	}

	for _, s := range deployment.Spec.Template.Volumes {
		pod.Spec.Template.Volumes = append(pod.Spec.Template.Volumes, s)
	}

	if err := p.storage.Pod().Insert(p.context, pod); err != nil {
		log.Errorf("Deployment: Pod insert: error %s", err)
		return nil, err
	}

	return pod, nil
}

// ListByNamespace returns pod list in selected namespace
func (p *Pod) ListByNamespace(namespace string) (map[string]*types.Pod, error) {
	log.V(logLevel).Debugf("Pod: get pod list by namespace %s", namespace)

	pods, err := p.storage.Pod().ListByNamespace(p.context, namespace)
	if err != nil {
		log.V(logLevel).Debugf("Pod: get Pod list by deployment id `%s` err: %s", namespace, err)
		return nil, err
	}

	return pods, nil
}

// ListByService returns pod list in selected service
func (p *Pod) ListByService(namespace, service string) (map[string]*types.Pod, error) {
	log.V(logLevel).Debugf("Pod: get pod list by service id %s/%s", namespace, service)

	pods, err := p.storage.Pod().ListByService(p.context, namespace, service)
	if err != nil {
		log.V(logLevel).Debugf("Pod: get Pod list by service id `%s` err: %s", namespace, service, err)
		return nil, err
	}

	return pods, nil
}

// ListByDeployment returns pod list in selected deployment
func (p *Pod) ListByDeployment(namespace, service, deployment string) (map[string]*types.Pod, error) {
	log.V(logLevel).Debugf("Pod: get pod list by id %s/%s/%s", namespace, service, deployment)

	pods, err := p.storage.Pod().ListByDeployment(p.context, namespace, service, deployment)
	if err != nil {
		log.V(logLevel).Debugf("Pod: get Pod list by deployment id `%s/%s/%s` err: %s",
			namespace, service, deployment, err)
		return nil, err
	}

	return pods, nil
}

// Schedule - schedule pod and bind node on it
func (p *Pod) Schedule(pod *types.Pod) (*types.Pod, error) {

	log.Debug("Deployment: Pod Schedule")

	//if err := p.storage.Pod().Bind(p.context, pod); err != nil {
	//	log.Errorf("Pod bind error: %s", err.Error())
	//	return nil, err
	//}

	isBind := len(pod.Meta.Node) != 0
	pod.State.Scheduled = isBind
	pod.State.Provision = isBind
	pod.State.Error = !isBind

	pod.Status.Steps[types.PodStepScheduled] = types.PodStep{
		Ready:     isBind,
		Timestamp: time.Now().UTC(),
	}

	if !isBind {
		pod.Status.Stage = types.PodStageError
		pod.Status.Message = "Can not bind node for pod"
	}

	if err := p.storage.Pod().SetState(p.context, pod); err != nil {
		return nil, err
	}

	return pod, nil
}

// SetState - set state for pod
func (p *Pod) SetState(pod *types.Pod) (*types.Pod, error) {

	log.Debugf("Set state for pod: %s", pod.Meta.Name)

	switch pod.Status.Stage {
	case types.PodStagePull:
		pod.MarkAsPull()
	case types.PodStageRunning:
		pod.MarkAsRunning()
	case types.PodStageStopped:
		pod.MarkAsStopped()
	case types.PodStageError:
		pod.MarkAsError(errors.New(pod.Status.Message))
	case types.PodStepDestroyed:
		pod.MarkAsDestroyed()
	}

	if pod.Status.Stage == types.PodStepDestroyed {
		if err := p.storage.Pod().Remove(p.context, pod); err != nil {
			log.Errorf("Pod remove err: %s", err.Error())
			return nil, err
		}
	} else {
		if err := p.storage.Pod().SetState(p.context, pod); err != nil {
			log.Errorf("Pod set state err: %s", err.Error())
			return nil, err
		}
	}

	return pod, nil
}

// Destroy pod
func (p *Pod) Destroy(ctx context.Context, pod *types.Pod) error {

	pod.Spec.State.Destroy = true
	if err := p.storage.Pod().SetSpec(p.context, pod); err != nil {
		log.Errorf("Mark pod for destroy error: %s", err.Error())
		return err
	}
	return nil
}

// Remove pod from storage
func (p *Pod) Remove(ctx context.Context, pod *types.Pod) error {
	if err := p.storage.Pod().Remove(p.context, pod); err != nil {
		log.Errorf("Mark pod for destroy error: %s", err.Error())
		return err
	}
	return nil
}

func NewPodModel(ctx context.Context, stg storage.Storage) IPod {
	return &Pod{ctx, stg}
}
