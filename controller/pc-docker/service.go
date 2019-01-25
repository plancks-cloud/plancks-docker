package pc_docker

import (
	"context"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/plancks-cloud/plancks-cloud/model"
	pc_model "github.com/plancks-cloud/plancks-docker/model"
	"log"
	"sort"
)

func CreateService(service *model.Service) (err error) {
	cli, err := client.NewEnvClient()
	ctx := context.Background()
	if err != nil {
		log.Printf("Error getting docker client environment: %s", err)
		return err
	}

	replicas := uint64(service.Replicas)

	spec := swarm.ServiceSpec{
		Annotations: swarm.Annotations{
			Name: service.Name,
		},
		Mode: swarm.ServiceMode{
			Replicated: &swarm.ReplicatedService{
				Replicas: &replicas,
			},
		},
		TaskTemplate: swarm.TaskSpec{
			ContainerSpec: swarm.ContainerSpec{
				Image: service.Image,
			},
			Resources: &swarm.ResourceRequirements{
				Limits: &swarm.Resources{
					MemoryBytes: int64(service.MemoryLimit * 1024 * 1024),
				},
			},
		},
	}

	_, err = cli.ServiceCreate(
		ctx,
		spec,
		types.ServiceCreateOptions{},
	)

	if err != nil {
		log.Printf("Error creating docker service: %s", err)
		return err
	}
	return err
}

//DockerServices gets all running docker services
func GetAllServices() (results []pc_model.ServiceState, err error) {

	cli, err := client.NewEnvClient()

	ctx := context.Background()

	if err != nil {
		log.Println(fmt.Sprintf("Error getting docker client environment: %s", err))
		return nil, err
	}

	services, err := cli.ServiceList(context.Background(), types.ServiceListOptions{})
	if err != nil {
		log.Println(fmt.Sprintf("Error getting docker client environment: %s", err))
		return nil, err
	}

	sort.Sort(pc_model.ByName(services))
	if len(services) > 0 {
		// only non-empty services and not quiet, should we call TaskList and NodeList api
		taskFilter := filters.NewArgs()
		for _, service := range services {
			taskFilter.Add("service", service.ID)
		}

		tasks, err := cli.TaskList(ctx, types.TaskListOptions{Filters: taskFilter})
		if err != nil {
			log.Println("Error getting tasks")
			return nil, err
		}

		nodes, err := cli.NodeList(ctx, types.NodeListOptions{})
		if err != nil {
			log.Println("Error getting nodes")
			return nil, err
		}

		info := TotalReplicas(services, nodes, tasks)

		for _, item := range info {
			results = append(results, item)
		}
	}
	return
}

//TotalReplicas returns the total number of replicas running for a service
func TotalReplicas(services []swarm.Service, nodes []swarm.Node, tasks []swarm.Task) map[string]pc_model.ServiceState {
	running := map[string]int{}
	tasksNoShutdown := map[string]int{}
	activeNodes := make(map[string]struct{})
	replicaState := make(map[string]pc_model.ServiceState)

	for _, n := range nodes {
		if n.Status.State != swarm.NodeStateDown {
			activeNodes[n.ID] = struct{}{}
		}
	}

	for _, task := range tasks {
		if task.DesiredState != swarm.TaskStateShutdown {
			tasksNoShutdown[task.ServiceID]++
		}
		if _, nodeActive := activeNodes[task.NodeID]; nodeActive && task.Status.State == swarm.TaskStateRunning {
			running[task.ServiceID]++
		}
	}

	for _, service := range services {
		if service.Spec.Mode.Replicated != nil && service.Spec.Mode.Replicated.Replicas != nil {
			replicaState[service.ID] = pc_model.ServiceState{
				ID:               service.ID,
				Name:             service.Spec.Name,
				Image:            service.Spec.TaskTemplate.ContainerSpec.Image,
				ReplicasRunning:  running[service.ID],
				ReplicasRequired: *service.Spec.Mode.Replicated.Replicas}
		}
	}
	return replicaState
}

func deleteServices(services []pc_model.ServiceState) (err error) {
	cli, err := client.NewEnvClient()
	ctx := context.Background()
	if err != nil {
		log.Printf("Error getting docker client environment: %s", err)
		return err
	}

	for _, service := range services {
		log.Printf("🔥  Removing service: %s", service.Name)
		cli.ServiceRemove(ctx, service.ID)
	}

	return
}