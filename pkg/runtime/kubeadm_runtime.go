// Copyright © 2021 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package runtime

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alibaba/sealer/pkg/runtime/kubeadm_types/v1beta2"

	"github.com/alibaba/sealer/utils"

	"github.com/alibaba/sealer/common"
	"github.com/alibaba/sealer/logger"
	v2 "github.com/alibaba/sealer/types/api/v2"
	"github.com/alibaba/sealer/utils/ssh"
)

type Config struct {
	Vlog         int
	VIP          string
	RegistryPort string
	// Clusterfile path and name, we needs read kubeadm config from Clusterfile
	Clusterfile     string
	APIServerDomain string
}

func newKubeadmRuntime(cluster *v2.Cluster, clusterfile string) (Interface, error) {
	k := &KubeadmRuntime{
		Cluster: cluster,
		Config: &Config{
			Clusterfile:     clusterfile,
			APIServerDomain: DefaultAPIserverDomain,
		},
		KubeadmConfig: &KubeadmConfig{},
	}
	k.setCertSANS(append([]string{"127.0.0.1", k.getAPIServerDomain(), k.getVIP()}, k.getMasterIPList()...))
	// TODO args pre checks
	if err := k.checkList(); err != nil {
		return nil, err
	}

	if logger.IsDebugModel() {
		k.Vlog = 6
	}
	return k, nil
}

func (k *KubeadmRuntime) checkList() error {
	return k.checkIPList()
}

func (k *KubeadmRuntime) checkIPList() error {
	if len(k.Spec.Hosts) == 0 {
		return fmt.Errorf("master hosts cannot be empty")
	}
	if len(k.Spec.Hosts[0].IPS) == 0 {
		return fmt.Errorf("master hosts ip cannot be empty")
	}
	return nil
}

func (k *KubeadmRuntime) getClusterName() string {
	return k.Cluster.Name
}

func (k *KubeadmRuntime) getHostSSHClient(hostIP string) (ssh.Interface, error) {
	return ssh.GetHostSSHClient(hostIP, k.Cluster)
}

func (k *KubeadmRuntime) getRootfs() string {
	return common.DefaultTheClusterRootfsDir(k.getClusterName())
}

func (k *KubeadmRuntime) getBasePath() string {
	return path.Join(common.DefaultClusterRootfsDir, k.Cluster.Name)
}

func (k *KubeadmRuntime) getMaster0IP() string {
	// already check ip list when new the runtime
	return k.Cluster.Spec.Hosts[0].IPS[0]
}

func (k *KubeadmRuntime) getDefaultKubeadmConfig() string {
	return filepath.Join(k.getRootfs(), "etc", "kubeadm.yml")
}

func (k *KubeadmRuntime) getCertPath() string {
	return path.Join(common.DefaultClusterRootfsDir, k.Cluster.Name, "pki")
}

func (k *KubeadmRuntime) getEtcdCertPath() string {
	return path.Join(common.DefaultClusterRootfsDir, k.Cluster.Name, "pki", "etcd")
}

func (k *KubeadmRuntime) getStaticFileDir() string {
	return path.Join(k.getRootfs(), "statics")
}

func (k *KubeadmRuntime) getSvcCIDR() string {
	return k.ClusterConfiguration.Networking.ServiceSubnet
}

func (k *KubeadmRuntime) setCertSANS(certSANS []string) {
	k.ClusterConfiguration.APIServer.CertSANs = utils.RemoveDuplicate(append(k.getCertSANS(), certSANS...))
}

func (k *KubeadmRuntime) getCertSANS() []string {
	return k.ClusterConfiguration.APIServer.CertSANs
}

func (k *KubeadmRuntime) getDNSDomain() string {
	if k.ClusterConfiguration.Networking.DNSDomain == "" {
		k.ClusterConfiguration.Networking.DNSDomain = "cluster.local"
	}
	return k.ClusterConfiguration.Networking.DNSDomain
}

func (k *KubeadmRuntime) getAPIServerDomain() string {
	return k.Config.APIServerDomain
}

func (k *KubeadmRuntime) getKubeVersion() string {
	return k.KubernetesVersion
}

func (k *KubeadmRuntime) getVIP() string {
	return DefaultVIP
}

func (k *KubeadmRuntime) getJoinToken() string {
	if k.Discovery.BootstrapToken == nil {
		return ""
	}
	return k.JoinConfiguration.Discovery.BootstrapToken.Token
}

func (k *KubeadmRuntime) setJoinToken(token string) {
	if k.Discovery.BootstrapToken == nil {
		k.Discovery.BootstrapToken = &v1beta2.BootstrapTokenDiscovery{}
	}
	k.Discovery.BootstrapToken.Token = token
}

func (k *KubeadmRuntime) getTokenCaCertHash() string {
	if k.Discovery.BootstrapToken == nil || len(k.Discovery.BootstrapToken.CACertHashes) == 0 {
		return ""
	}
	return k.Discovery.BootstrapToken.CACertHashes[0]
}

func (k *KubeadmRuntime) setTokenCaCertHash(tokenCaCertHash []string) {
	if k.Discovery.BootstrapToken == nil {
		k.Discovery.BootstrapToken = &v1beta2.BootstrapTokenDiscovery{}
	}
	k.Discovery.BootstrapToken.CACertHashes = tokenCaCertHash
}

func (k *KubeadmRuntime) getCertificateKey() string {
	if k.JoinConfiguration.ControlPlane == nil {
		return ""
	}
	return k.JoinConfiguration.ControlPlane.CertificateKey
}

func (k *KubeadmRuntime) setInitCertificateKey(certificateKey string) {
	k.CertificateKey = certificateKey
}

func (k *KubeadmRuntime) setAPIServerEndpoint(endpoint string) {
	k.JoinConfiguration.Discovery.BootstrapToken.APIServerEndpoint = endpoint
}

func (k *KubeadmRuntime) setInitAdvertiseAddress(advertiseAddress string) {
	k.InitConfiguration.LocalAPIEndpoint.AdvertiseAddress = advertiseAddress
}

func (k *KubeadmRuntime) setJoinAdvertiseAddress(advertiseAddress string) {
	if k.JoinConfiguration.ControlPlane == nil {
		k.JoinConfiguration.ControlPlane = &v1beta2.JoinControlPlane{}
	}
	k.JoinConfiguration.ControlPlane.LocalAPIEndpoint.AdvertiseAddress = advertiseAddress
}

func (k *KubeadmRuntime) cleanJoinLocalAPIEndPoint() {
	k.JoinConfiguration.ControlPlane = nil
}

func (k *KubeadmRuntime) setControlPlaneEndpoint(endpoint string) {
	k.ControlPlaneEndpoint = endpoint
}

func (k *KubeadmRuntime) setCgroupDriver(cGroup string) {
	k.KubeletConfiguration.CgroupDriver = cGroup
}

func (k *KubeadmRuntime) getMasterIPList() (masters []string) {
	return k.getHostsIPByRole(common.MASTER)
}

func (k *KubeadmRuntime) getNodesIPList() (nodes []string) {
	return k.getHostsIPByRole(common.NODE)
}

func (k *KubeadmRuntime) getHostsIPByRole(role string) (nodes []string) {
	for _, host := range k.Spec.Hosts {
		if utils.InList(role, host.Roles) {
			nodes = append(nodes, host.IPS...)
		}
	}

	return
}

func getEtcdEndpointsWithHTTPSPrefix(masters []string) string {
	var tmpSlice []string
	for _, ip := range masters {
		tmpSlice = append(tmpSlice, fmt.Sprintf("https://%s:2379", utils.GetHostIP(ip)))
	}
	return strings.Join(tmpSlice, ",")
}

func (k *KubeadmRuntime) WaitSSHReady(tryTimes int, hosts ...string) error {
	errCh := make(chan error, len(hosts))
	defer close(errCh)

	var wg sync.WaitGroup
	for _, h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			for i := 0; i < tryTimes; i++ {
				ssh, err := k.getHostSSHClient(host)
				if err != nil {
					return
				}

				err = ssh.Ping(host)
				if err == nil {
					return
				}
				time.Sleep(time.Duration(i) * time.Second)
			}
			err := fmt.Errorf("wait for [%s] ssh ready timeout, ensure that the IP address or password is correct", host)
			errCh <- err
		}(h)
	}
	wg.Wait()
	return ReadChanError(errCh)
}
