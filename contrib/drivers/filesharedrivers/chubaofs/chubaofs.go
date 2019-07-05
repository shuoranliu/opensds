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
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	log "github.com/golang/glog"
	. "github.com/opensds/opensds/contrib/drivers/utils/config"
	. "github.com/opensds/opensds/pkg/model"
	pb "github.com/opensds/opensds/pkg/model/proto"
	"github.com/opensds/opensds/pkg/utils/config"
	uuid "github.com/satori/go.uuid"
)

const (
	DefaultConfPath = "/etc/opensds/driver/chubaofs.yaml"
	NamePrefix      = "chubaofs"
)

const (
	KMountPoint = "mountPoint"
	KVolumeName = "volName"
	KMasterAddr = "masterAddr"
	KLogDir     = "logDir"
	KWarnLogDir = "warnLogDir"
	KLogLevel   = "logLevel"
	KOwner      = "owner"
	KProfPort   = "profPort"
)

const (
	defaultLogLevel = "error"
	defaultOwner    = "chubaofs"
	defaultProfPort = "10094"
)

const (
	clientConfigFileName = "client.json"
	clientCmdName        = "cfs-client"
)

type ClusterInfo struct {
	Name       string   `yaml:"name"`
	MasterAddr []string `yaml:"masterAddr"`
}

type RuntimeEnv struct {
	MntPoint   string `yaml:"mntPoint"`
	ClientPath string `yaml:"clientPath"`
	LogLevel   string `yaml:"logLevel"`
	Owner      string `yaml:"owner"`
	ProfPort   string `yaml:"profPort"`
}

type Config struct {
	ClusterInfo `yaml:"clusterInfo"`
	RuntimeEnv  `yaml:"runtimeEnv"`

	Pool map[string]PoolProperties `yaml:"pool,flow"`
}

type Driver struct {
	conf *Config
}

func (d *Driver) Setup() error {
	conf := &Config{}
	path := config.CONF.OsdsDock.Backends.Chubaofs.ConfigPath
	if "" == path {
		path = DefaultConfPath
	}
	_, err := Parse(conf, path)

	if conf.MntPoint == "" || conf.ClientPath == "" {
		return errors.New(fmt.Sprintf("chubaofs: lack of necessary config, mntPoint(%v) clientPath(%v)", conf.MntPoint, conf.ClientPath))
	}

	d.conf = conf
	return err
}

func (d *Driver) Unset() error {
	return nil
}

func (d *Driver) CreateFileShare(opt *pb.CreateFileShareOpts) (*FileShareSpec, error) {
	log.Info("CreateFileShare ...")

	// check runtime environments
	fi, err := os.Stat(d.conf.ClientPath)
	if err != nil || !fi.Mode().IsDir() {
		return nil, errors.New(fmt.Sprintf("chubaofs: invalid client path", d.conf.ClientPath))
	}

	clientCmd := path.Join(d.conf.ClientPath, "bin", clientCmdName)
	clientConf := path.Join(d.conf.ClientPath, "conf", opt.GetId())
	clientLog := path.Join(d.conf.ClientPath, "log", opt.GetId())
	clientWarnLog := path.Join(d.conf.ClientPath, "warnlog", opt.GetId())

	if err = os.MkdirAll(clientConf, os.ModeDir); err != nil {
		return nil, errors.New(fmt.Sprintf("chubaofs: failed to mkdir %v", clientConf))
	}

	if err = os.MkdirAll(clientLog, os.ModeDir); err != nil {
		return nil, errors.New(fmt.Sprintf("chubaofs: failed to mkdir %v", clientLog))
	}

	if err = os.MkdirAll(clientWarnLog, os.ModeDir); err != nil {
		return nil, errors.New(fmt.Sprintf("chubaofs: failed to mkdir %v", clientWarnLog))
	}

	fi, err = os.Stat(d.conf.MntPoint)
	if err != nil || !fi.Mode().IsDir() {
		return nil, errors.New(fmt.Sprintf("chubaofs: invalid mount point %v", d.conf.MntPoint))
	}

	fsMntPoint := path.Join(d.conf.MntPoint, opt.GetId())
	if err = os.MkdirAll(fsMntPoint, os.ModeDir); err != nil {
		return nil, errors.New(fmt.Sprintf("chubaofs: failed to mkdir %v", fsMntPoint))
	}

	// do create volume

	leader, err := getClusterInfo(d.conf.MasterAddr[0])
	if err != nil {
		return nil, err
	}

	err = createVolume(leader, opt.GetId(), opt.Size)
	if err != nil {
		return nil, err
	}

	// do mount

	mntConfig := make(map[string]interface{})
	mntConfig[KMountPoint] = fsMntPoint
	mntConfig[KVolumeName] = opt.GetId()
	mntConfig[KMasterAddr] = strings.Join(d.conf.MasterAddr, ",")
	mntConfig[KLogDir] = clientLog
	mntConfig[KWarnLogDir] = clientWarnLog
	if d.conf.LogLevel != "" {
		mntConfig[KLogLevel] = d.conf.LogLevel
	} else {
		mntConfig[KLogLevel] = defaultLogLevel
	}
	if d.conf.Owner != "" {
		mntConfig[KOwner] = d.conf.Owner
	} else {
		mntConfig[KOwner] = defaultOwner
	}
	if d.conf.ProfPort != "" {
		mntConfig[KProfPort] = d.conf.ProfPort
	} else {
		mntConfig[KProfPort] = defaultProfPort
	}

	data, err := json.MarshalIndent(mntConfig, "", "    ")
	if err != nil {
		log.Errorf("chubaofs: failed to generate client config file, err(%v)", err)
		return nil, err
	}

	clientConfFile := path.Join(clientConf, clientConfigFileName)

	_, err = generateFile(clientConfFile, data)
	if err != nil {
		log.Errorf("chubaofs: failed to generate client config file, err(%v)", err)
		return nil, err
	}

	cmd := exec.Command(clientCmd, "-c", clientConfFile)
	if msg, err := cmd.CombinedOutput(); err != nil {
		log.Errorf("chubaofs: failed to start client daemon, msg(%v) err(%v)", msg, err)
		return nil, err
	}

	log.Infof("Start client daemon successful: %v -c %v", clientCmd, clientConfFile)

	locations := make([]string, 1)
	locations[0] = fsMntPoint

	fshare := &FileShareSpec{
		BaseModel: &BaseModel{
			Id: opt.GetId(),
		},
		Name:             opt.GetName(),
		Size:             opt.GetSize(),
		Description:      opt.GetDescription(),
		AvailabilityZone: opt.GetAvailabilityZone(),
		PoolId:           opt.GetPoolId(),
		ExportLocations:  locations,
		Metadata: map[string]string{
			KMountPoint: fsMntPoint,
			KVolumeName: opt.GetId(),
		},
	}
	return fshare, nil
}

func (d *Driver) DeleteFileShare(opts *pb.DeleteFileShareOpts) error {
	// TODO
	return nil
}

func (d *Driver) CreateFileShareSnapshot(opts *pb.CreateFileShareSnapshotOpts) (*FileShareSnapshotSpec, error) {
	return nil, &NotImplementError{"CreateFileShareSnapshot not implemented yet"}
}

func (d *Driver) DeleteFileShareSnapshot(opts *pb.DeleteFileShareSnapshotOpts) error {
	return &NotImplementError{"DeleteFileShareSnapshot not implemented yet"}
}

func (d *Driver) CreateFileShareAcl(opts *pb.CreateFileShareAclOpts) (*FileShareAclSpec, error) {
	return nil, &NotImplementError{"CreateFileShareAcl not implemented yet"}
}

func (d *Driver) DeleteFileShareAcl(opts *pb.DeleteFileShareAclOpts) error {
	return &NotImplementError{"DeleteFileShareAcl not implemented yet"}
}

func (d *Driver) ListPools() ([]*StoragePoolSpec, error) {
	pools := make([]*StoragePoolSpec, 0)
	for name, prop := range d.conf.Pool {
		pool := &StoragePoolSpec{
			BaseModel: &BaseModel{
				Id: uuid.NewV5(uuid.NamespaceOID, name).String(),
			},
			Name:             name,
			TotalCapacity:    200, // FIXME
			FreeCapacity:     200, // FIXME
			StorageType:      prop.StorageType,
			Extras:           prop.Extras,
			AvailabilityZone: prop.AvailabilityZone,
		}
		if pool.AvailabilityZone == "" {
			pool.AvailabilityZone = "default"
		}
		pools = append(pools, pool)

	}
	return pools, nil
}
