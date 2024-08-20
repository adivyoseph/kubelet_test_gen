/*
	This program reads jason file
	builds pods specs for the tester
	builds config files for each test
*/

package main

import (
	"fmt"
	"io/ioutil"
	"os"

	//"gopkg.in/yaml.v2"
	//"os"
	"encoding/json"
	"log"
)

type TopologyConfigStruct struct {
	Name       string
	Sockets    int
	SockNuma   int
	L3GroupPer int
	CoresPerL3 int
	SmtOn      bool
}

type ContainerDefinitionStruct struct {
	Name  string
	Sizes []int
}

type PodDefinitionStruct struct {
	Name       string
	Replicas   int
	Containers []ContainerDefinitionStruct
}

type ConfigStruct struct {
	Topology      TopologyConfigStruct
	ReservedCores []int
	PodSet        []PodDefinitionStruct
}

type PodEntry struct {
	podIndex          int
	containerZeroSize int
	totalSize         int
}

type SchedPods struct {
	totalSize int
	pods      []PodEntry
}

type AppState struct {
	config    ConfigStruct
	totalCpus int
	runs      []SchedPods
}

//var podSet PodSetStruct

func main() {
	appState := AppState{}
	appState.totalCpus = appState.config.getConfig("test.json") //builds topology
	appState.buildPodSets()
	appState.buildTests()

}

func (config *ConfigStruct) getConfig(fileName string) int {
	yamlFile, err := ioutil.ReadFile(fileName)
	if err != nil {
		fmt.Printf("%v err   #%v \n", fileName, err)
		return 0
	}

	err = json.Unmarshal(yamlFile, config)
	if err != nil {
		fmt.Printf("Unmarshal: %v\n", err)
	}
	fmt.Printf("Topology.Name        %v\n", config.Topology.Name)
	fmt.Printf("Topology.Sockets     %v\n", config.Topology.Sockets)
	fmt.Printf("Topology.SockNuma    %v\n", config.Topology.SockNuma)
	fmt.Printf("Topology.L3GroupsPer %v\n", config.Topology.L3GroupPer)
	fmt.Printf("Topology.CoresPerL3  %v\n", config.Topology.CoresPerL3)
	fmt.Printf("Topology.SmtOn       %v\n", config.Topology.SmtOn)

	fmt.Printf("ReservedCores %v\n", config.ReservedCores)
	fmt.Printf("PodSet %v\n", config.PodSet)

	totalCoresPerSocket := config.Topology.SockNuma * config.Topology.L3GroupPer * config.Topology.CoresPerL3
	fmt.Printf("totalCoresPerSocket %d\n", totalCoresPerSocket)
	totalCpus := config.Topology.Sockets * totalCoresPerSocket
	if config.Topology.SmtOn {
		totalCpus = totalCpus * 2
	}
	fmt.Printf("totalCpus %d\n", totalCpus)
	fmt.Printf("\n")
	return totalCpus
}

func (p *SchedPods) addPod(config *ConfigStruct, podIndex int, index int) {
	podEntry := PodEntry{}
	podEntry.podIndex = podIndex
	podEntry.containerZeroSize = config.PodSet[podIndex].Containers[0].Sizes[index]
	podEntry.totalSize = config.PodSet[podIndex].Containers[0].Sizes[index]
	if len(config.PodSet[podIndex].Containers) > 1 {
		//add side car
		podEntry.totalSize = podEntry.totalSize + config.PodSet[podIndex].Containers[1].Sizes[0]
		//fmt.Printf("%s[%d]%d:%d,", config.PodSet[podIndex].Name, index, config.PodSet[podIndex].Containers[0].Sizes[index], config.PodSet[podIndex].Containers[1].Sizes[0])
	} else {
		//fmt.Printf("%s[%d]%d:0,", config.PodSet[podIndex].Name, index, config.PodSet[podIndex].Containers[0].Sizes[index])
	}
	p.totalSize = p.totalSize + podEntry.totalSize

	p.pods = append(p.pods, podEntry)
}

func (as *AppState) buildTests() {

	if _, err := os.Stat("./pods"); err != nil {
		err := os.Mkdir("./pods", 0755)
		if err != nil {
			fmt.Printf("mdir ./pods faled\n")
			return
		}
	}

	if _, err := os.Stat("./test"); err != nil {
		err := os.Mkdir("./test", 0755)
		if err != nil {
			fmt.Printf("mkdir ./test faled\n")
			return
		}
	}

	testBaseDir := fmt.Sprintf("./test/%s", as.config.Topology.Name)
	if _, err := os.Stat(testBaseDir); err != nil {
		err := os.Mkdir(testBaseDir, 0755)
		if err != nil {
			fmt.Printf("mkdir %s faled\n", testBaseDir)
			return
		}
	}

	for runIndex, runPodList := range as.runs {
		numPods := len(runPodList.pods)
		// build pods
		for _, podEntry := range runPodList.pods {
			podIndex := podEntry.podIndex
			fmt.Printf("POD name %s replicas %d\n", as.config.PodSet[podIndex].Name, as.config.PodSet[podIndex].Replicas)
			for replica := 0; replica < as.config.PodSet[podIndex].Replicas; replica++ {
				podName := fmt.Sprintf("%s-%d-%d", as.config.PodSet[podIndex].Name, podEntry.totalSize, runIndex)
				fileName := fmt.Sprintf("./pods/%s.yaml", podName)

				_, error := os.Stat(fileName)
				// check if error is "file not exists"
				if !os.IsNotExist(error) {
					err := os.Remove(fileName)
					if err != nil {
						log.Fatal(err)
					}
				}

				f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0644)
				if err != nil {
					log.Fatal(err)
				}
				if _, err := f.Write([]byte("---\n")); err != nil {
					log.Fatal(err)
				}
				if _, err := f.Write([]byte(fmt.Sprintf("apiVersion: v1\nkind: Pod\nmetadata:\n  name: %s\n", podName))); err != nil {
					log.Fatal(err)
				}
				if _, err := f.Write([]byte("spec:\n  containers:\n")); err != nil {
					log.Fatal(err)
				}
				for container := 0; container < len(as.config.PodSet[podIndex].Containers); container++ {
					if container == 0 {
						containerName := fmt.Sprintf("%s-%d-%d", as.config.PodSet[podIndex].Containers[0].Name, podEntry.containerZeroSize, runIndex)
						if _, err := f.Write([]byte(fmt.Sprintf("      - name: %s # Main container\n         image: nginx # Use the nginx image\n", containerName))); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte("         resources:\n            requests:\n               memory: \"1064Mi\"\n")); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte(fmt.Sprintf("               cpu: \"%d\"\n            limits:\n               memory: \"1064Mi\"\n",
							podEntry.containerZeroSize))); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte(fmt.Sprintf("               cpu: \"%d\"\n         ports:\n",
							podEntry.containerZeroSize))); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte("						  - containerPort: 80 # Expose port 80\n         command: [\"/bin/sh\"] # Override the default command\n")); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte("         args: [\"-c\", \"while true; do echo \\\"$(date) Hello from nginx\\\"; sleep 1; done | tee /var/log/nginx/access.log\"]\n")); err != nil {
							log.Fatal(err)
						}
					} else { //sidecar
						containerName := fmt.Sprintf("%s-%d-%d", as.config.PodSet[podIndex].Containers[1].Name, podEntry.containerZeroSize, runIndex)
						if _, err := f.Write([]byte(fmt.Sprintf("     - name: %s # # Sidecar container\n         image: alpine/socat # Use the alpine/socat image\n", containerName))); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte("         resources:\n            requests:\n               memory: \"64Mi\"\n")); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte(fmt.Sprintf("               cpu: \"%d\"\n            limits:\n               memory: \"64Mi\"\n",
							podEntry.totalSize-podEntry.containerZeroSize))); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte(fmt.Sprintf("               cpu: \"%d\"\n         ports:\n",
							podEntry.totalSize-podEntry.containerZeroSize))); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte("						  - containerPort: 8080 # Expose port 8080\n         command: [\"socat\"] # Override the default command\n")); err != nil {
							log.Fatal(err)
						}
						if _, err := f.Write([]byte("         args: [\"-v\", \"TCP-LISTEN:8080,fork,reuseaddr\", \"EXEC:\\\"kubectl logs web-server-7f9f8c4b9-6xq8w -c nginx\\\"\"]\n")); err != nil {
							log.Fatal(err)
						}
						//					if _, err := f.Write([]byte("         # Run socat to listen on port 8080 and execute kubectl logs to return the logs from the main container\n")); err != nil {
						//						log.Fatal(err)
						//					}

					}

				}
				if err := f.Close(); err != nil {
					log.Fatal(err)
				}
			}

		}

		orderList := []SchedPods{}
		for outer := 0; outer < numPods; outer++ {
			newPodList := SchedPods{}
			orderList = append(orderList, newPodList)
			for inner := 0; inner < numPods; inner++ {
				newPodSet := PodEntry{}
				orderList[outer].pods = append(orderList[outer].pods, newPodSet)
			}
		}

		for i := 0; i < numPods; i++ {
			s := i
			for x := 0; x < numPods; x++ {
				orderList[i].pods[x] = runPodList.pods[s]
				s++
				if s >= numPods {
					s = 0
				}
			}
		}

		//proces orderlist
		for orderEntry := 0; orderEntry < numPods; orderEntry++ {
			//podIndex := runPodList.pods[i].podIndex
			//totalSize := runPodList.pods[i].totalSize
			//podName := as.config.PodSet[podIndex].Name
			//podReplicas := as.config.PodSet[podIndex].Replicas
			testName := fmt.Sprintf("run-%d-%d", runIndex, orderEntry)
			fileName := fmt.Sprintf("test/%s/%s/config.yml", as.config.Topology.Name, testName)

			testBaseDir = fmt.Sprintf("./test/%s/%s", as.config.Topology.Name, testName)
			if _, err := os.Stat(testBaseDir); err != nil {
				err := os.Mkdir(testBaseDir, 0755)
				if err != nil {
					fmt.Printf("mkdir %s faled\n", testBaseDir)
					return
				}
			}

			_, error := os.Stat(fileName)
			// check if error is "file not exists"
			if !os.IsNotExist(error) {
				err := os.Remove(fileName)
				if err != nil {
					log.Fatal(err)
				}
			}
			//ok to build file now
			f, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte(fmt.Sprintf("name: \"%s\"\n", testName))); err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte(fmt.Sprintf("version: \"%s\"\n", "1.0.0"))); err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte(fmt.Sprintf("active: \"%s\"\n", "yes"))); err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte(fmt.Sprintf("description: \"%s\"\n", " "))); err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte(fmt.Sprintf("add: \"%s\"\n", " "))); err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte(fmt.Sprintf("remove: \"%s\"\n", " "))); err != nil {
				log.Fatal(err)
			}
			if _, err := f.Write([]byte("run: \" ")); err != nil {
				log.Fatal(err)
			}
			for pod := 0; pod < numPods; pod++ {
				podIndex := orderList[orderEntry].pods[pod].podIndex
				totalSize := orderList[orderEntry].pods[pod].totalSize
				podName := fmt.Sprintf("%s-%d-%d", as.config.PodSet[podIndex].Name, totalSize, runIndex)

				if _, err := f.Write([]byte(fmt.Sprintf("%s, ", podName))); err != nil {
					log.Fatal(err)
				}

			}
			if _, err := f.Write([]byte("\"\n")); err != nil {
				log.Fatal(err)
			}

			if err := f.Close(); err != nil {
				log.Fatal(err)
			}

		}

	}

}

func (as *AppState) buildPodSets() {

	numPods := len(as.config.PodSet)
	fmt.Printf("\tnumPods %v\n", numPods)
	width := 0
	for _, pod := range as.config.PodSet {
		for _, container := range pod.Containers {
			if len(container.Sizes) > width {
				//fmt.Fprintf(logFile, "pod %s width %d\n", pod.Name, len(container.Sizes))
				width = len(container.Sizes)
			}
		}

	}
	work := len(as.config.ReservedCores)
	if as.config.Topology.SmtOn {
		work *= 2
	}

	fmt.Printf("totalAvailableCpus %d (-2)\n", as.totalCpus-work-2)

	fmt.Printf("\twidth %d\n", width)

	for poda := 0; poda < width; poda++ {

		for podb := 0; podb < width; podb++ {

			for podc := 0; podc < width; podc++ {

				for podd := 0; podd < width; podd++ {

					for pode := 0; pode < len(as.config.PodSet[4].Containers[0].Sizes); pode++ {
						podsNew := SchedPods{}
						for rep := 0; rep < as.config.PodSet[0].Replicas; rep++ {
							podsNew.addPod(&as.config, 0, poda)
						}
						podsNew.addPod(&as.config, 1, podb)
						podsNew.addPod(&as.config, 2, podc)
						podsNew.addPod(&as.config, 3, podd)
						podsNew.addPod(&as.config, 4, pode)
						//fmt.Printf("\n")
						if podsNew.totalSize > (as.totalCpus - work - 2) {
							fmt.Printf(" buildPodSets too big %d %v\n", podsNew.totalSize, podsNew)
							continue
						}
						fmt.Printf("podsNew = %v\n", podsNew)
						as.runs = append(as.runs, podsNew)
					}
				}
			}
		}
	}
}
