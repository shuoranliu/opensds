// Copyright (c) 2019 The OpenSDS Authors.
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

package chubaofs

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"

	log "github.com/golang/glog"
	. "github.com/opensds/opensds/contrib/drivers/utils/config"
	. "github.com/opensds/opensds/pkg/model"
	pb "github.com/opensds/opensds/pkg/model/proto"
	"github.com/opensds/opensds/pkg/utils/config"
)

const (
	DefaultConfPath       = "/etc/chubaofs/driver/chubaofs.yaml"
	DefaultClientConfPath = "/etc/chubaofs/driver/client.json"
	NamePrefix            = "chubaofs"
)

const (
	KMountPoint = "mountPoint"
	KVolumeName = "volName"
	KMasterAddr = "masterAddr"
	KLogDir     = "logDir"
	KLogLevel   = "logLevel"
	KOwner      = "owner"
	KProfPort   = "profPort"
)

type ClusterInfo struct {
	Name       string   `yaml:"name"`
	MasterAddr []string `yaml:"masterAddr"`
}

type Config struct {
	ClusterInfo `yaml:"clusterInfo"`
	Pool        map[string]PoolProperties `yaml:"pool,flow"`
}

type Driver struct {
	conf *Config
}

func (d *Driver) Setup() error {
	conf := &Config{}
	d.conf = conf
	path := config.CONF.OsdsDock.Backends.Chubaofs.ConfigPath
	if "" == path {
		path = DefaultConfPath
	}
	_, err := Parse(conf, path)
	return err
}

func (d *Driver) Unset() error {
	return nil
}

func (d *Driver) CreateVolume(opt *pb.CreateVolumeOpts) (*VolumeSpec, error) {
	log.Info("CreateVolume ...")

	leader, err := getClusterInfo(d.conf.MasterAddr[0])
	if err != nil {
		return nil, err
	}

	err = createVolume(leader, opt.GetId(), opt.Size)
	if err != nil {
		return nil, err
	}

	return &VolumeSpec{
		BaseModel: &BaseModel{
			Id: opt.GetId(),
		},
		Name:             opt.GetName(),
		Size:             opt.Size,
		Description:      opt.GetDescription(),
		AvailabilityZone: opt.GetAvailabilityZone(),
		PoolId:           opt.GetPoolId(),
		Metadata:         nil,
	}, nil
}

func (d *Driver) PullVolume(volIdentifier string) (*VolumeSpec, error) {
	return nil, nil
}

func (d *Driver) DeleteVolume(opt *pb.DeleteVolumeOpts) error {
	return nil
}

func (d *Driver) ExtendVolume(opt *pb.ExtendVolumeOpts) (*VolumeSpec, error) {
	return nil, nil
}

func (d *Driver) InitializeConnection(opt *pb.CreateVolumeAttachmentOpts) (*ConnectionInfo, error) {
	log.Info("InitializeConnection ...")

	mountPoint, ok := opt.GetMetadata()[KMountPoint]
	if !ok {
		err := errors.New("can't find 'mountPoint' in metadata")
		log.Error(err)
		return nil, err
	}

	mntConfig := make(map[string]interface{})
	mntConfig[KMountPoint] = mountPoint
	mntConfig[KVolumeName] = opt.Id
	mntConfig[KMasterAddr] = strings.Join(d.conf.MasterAddr, ",")
	// FIXME: make configurable
	mntConfig[KLogDir] = "/export/Logs/chubaofs/"
	mntConfig[KLogLevel] = "info"
	mntConfig[KOwner] = "chubaofs"
	mntConfig[KProfPort] = "10094"

	data, err := json.MarshalIndent(mntConfig, "", "    ")
	if err != nil {
		log.Errorf("chubaofs: failed to generate client config file, err(%v)", err)
		return nil, err
	}

	// FIXME: client conf path
	_, err = generateFile(DefaultClientConfPath, data)
	if err != nil {
		log.Errorf("chubaofs: failed to generate client config file, err(%v)", err)
		return nil, err
	}

	go func() {
		log.Infof("Run client /usr/bin/cfs-client -c %v", DefaultClientConfPath)
		cmd := exec.Command("/usr/bin/cfs-client", "-c", DefaultClientConfPath)
		if e := cmd.Run(); e != nil {
			log.Errorf("chubaofs: failed to run client, err(%v)", err)
			return
		}
	}()

	// FIXME: handle error

	return &ConnectionInfo{
		DriverVolumeType: ISCSIProtocol,
		ConnectionData:   map[string]interface{}{},
	}, nil
}

func (d *Driver) TerminateConnection(opt *pb.DeleteVolumeAttachmentOpts) error {
	log.Info("TerminateConnection...")
	// FIXME: Umount
	return nil
}

func (d *Driver) CreateSnapshot(opt *pb.CreateVolumeSnapshotOpts) (*VolumeSnapshotSpec, error) {
	return nil, nil
}

func (d *Driver) PullSnapshot(snapIdentifier string) (*VolumeSnapshotSpec, error) {
	return nil, nil
}

func (d *Driver) DeleteSnapshot(opt *pb.DeleteVolumeSnapshotOpts) error {
	return nil
}

func (d *Driver) ListPools() ([]*StoragePoolSpec, error) {
	log.Info("ListPools ...")
	var pols []*StoragePoolSpec
	// TODO: Get all pools from actual storage backend, and
	// filter them by items that is configured in  /etc/opensds/driver/newdriver.yaml
	return pols, nil
}

// The interfaces blow are optional, so implement it or not depends on you.
func (d *Driver) InitializeSnapshotConnection(opt *pb.CreateSnapshotAttachmentOpts) (*ConnectionInfo, error) {
	return nil, &NotImplementError{S: "method InitializeSnapshotConnection has not been implemented yet."}
}

func (d *Driver) TerminateSnapshotConnection(opt *pb.DeleteSnapshotAttachmentOpts) error {
	return &NotImplementError{S: "method TerminateSnapshotConnection has not been implemented yet."}
}

func (d *Driver) CreateVolumeGroup(opt *pb.CreateVolumeGroupOpts) (*VolumeGroupSpec, error) {
	return nil, &NotImplementError{S: "method CreateVolumeGroup has not been implemented yet."}
}

func (d *Driver) UpdateVolumeGroup(opt *pb.UpdateVolumeGroupOpts) (*VolumeGroupSpec, error) {
	return nil, &NotImplementError{S: "method UpdateVolumeGroup has not been implemented yet."}
}

func (d *Driver) DeleteVolumeGroup(opt *pb.DeleteVolumeGroupOpts) error {
	return nil
}
