// SPDX-License-Identifier: Apache-2.0
// Copyright Authors of Cilium

//go:build !privileged_tests
// +build !privileged_tests

package eni

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gopkg.in/check.v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorOption "github.com/cilium/cilium/operator/option"
	ec2mock "github.com/cilium/cilium/pkg/aws/ec2/mock"
	eniTypes "github.com/cilium/cilium/pkg/aws/eni/types"
	"github.com/cilium/cilium/pkg/aws/types"
	"github.com/cilium/cilium/pkg/checker"
	"github.com/cilium/cilium/pkg/ipam"
	metricsmock "github.com/cilium/cilium/pkg/ipam/metrics/mock"
	ipamTypes "github.com/cilium/cilium/pkg/ipam/types"
	v2 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2"
	"github.com/cilium/cilium/pkg/testutils"
)

var (
	testSubnet = &ipamTypes.Subnet{
		ID:                 "s-1",
		AvailabilityZone:   "us-west-1",
		VirtualNetworkID:   "vpc-1",
		AvailableAddresses: 200,
		Tags:               ipamTypes.Tags{"k": "v"},
	}
	testVpc = &ipamTypes.VirtualNetwork{
		ID:          "vpc-1",
		PrimaryCIDR: "10.10.0.0/16",
	}
	testSecurityGroups = []*types.SecurityGroup{
		{
			ID:    "sg-1",
			VpcID: "vpc-1",
			Tags:  ipamTypes.Tags{"test-sg-1": "yes"},
		},
		{
			ID:    "sg-2",
			VpcID: "vpc-1",
			Tags:  ipamTypes.Tags{"test-sg-2": "yes"},
		},
	}
	k8sapi     = &k8sMock{}
	metricsapi = metricsmock.NewMockMetrics()
)

func (e *ENISuite) SetUpTest(c *check.C) {
	metricsapi = metricsmock.NewMockMetrics()
}

func (e *ENISuite) TearDownTest(c *check.C) {
	metricsapi = nil
}

func (e *ENISuite) TestGetNodeNames(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testGetNodeNames-1", eniID1)
	c.Assert(err, check.IsNil)
	eniID2, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testGetNodeNames-2", eniID2)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())

	mngr.Update(newCiliumNode("node1", "i-testGetNodeNames-1", "m4.large", "us-west-1", "vpc-1", 0, 0, 0, 0))

	names := mngr.GetNames()
	c.Assert(len(names), check.Equals, 1)
	c.Assert(names[0], check.Equals, "node1")

	mngr.Update(newCiliumNode("node2", "i-testGetNodeNames-2", "m4.large", "us-west-1", "vpc-1", 0, 0, 0, 0))

	names = mngr.GetNames()
	c.Assert(len(names), check.Equals, 2)

	mngr.Delete("node1")

	names = mngr.GetNames()
	c.Assert(len(names), check.Equals, 1)
	c.Assert(names[0], check.Equals, "node2")
}

func (e *ENISuite) TestNodeManagerGet(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerGet-1", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())

	mngr.Update(newCiliumNode("node1", "i-testNodeManagerGet-1", "m4.large", "us-west-1", "vpc-1", 0, 0, 0, 0))

	c.Assert(mngr.Get("node1"), check.Not(check.IsNil))
	c.Assert(mngr.Get("node2"), check.IsNil)

	mngr.Delete("node1")
	c.Assert(mngr.Get("node1"), check.IsNil)
	c.Assert(mngr.Get("node2"), check.IsNil)
}

type k8sMock struct{}

func (k *k8sMock) Create(node *v2.CiliumNode) (*v2.CiliumNode, error) {
	return nil, nil
}

func (k *k8sMock) Update(origNode, node *v2.CiliumNode) (*v2.CiliumNode, error) {
	return nil, nil
}

func (k *k8sMock) UpdateStatus(origNode, node *v2.CiliumNode) (*v2.CiliumNode, error) {
	return nil, nil
}

func (k *k8sMock) Get(node string) (*v2.CiliumNode, error) {
	return &v2.CiliumNode{}, nil
}

func (k *k8sMock) Delete(node string) error {
	return nil
}

func newCiliumNode(node, instanceID, instanceType, az, vpcID string, firstInterfaceIndex, preAllocate, minAllocate, maxAllocate int) *v2.CiliumNode {
	cn := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{Name: node, Namespace: "default"},
		Spec: v2.NodeSpec{
			InstanceID: instanceID,
			ENI: eniTypes.ENISpec{
				InstanceType:        instanceType,
				FirstInterfaceIndex: &firstInterfaceIndex,
				AvailabilityZone:    az,
				VpcID:               vpcID,
			},
			IPAM: ipamTypes.IPAMSpec{
				Pool:        ipamTypes.AllocationMap{},
				PreAllocate: preAllocate,
				MinAllocate: minAllocate,
				MaxAllocate: maxAllocate,
			},
		},
		Status: v2.NodeStatus{
			IPAM: ipamTypes.IPAMStatus{
				Used: ipamTypes.AllocationMap{},
			},
		},
	}

	return cn
}

func newCiliumNodeWithSGTags(node, instanceID, instanceType, az, vpcID string, sgTags map[string]string, firstInterfaceIndex, preAllocate, minAllocate, maxAllocate int) *v2.CiliumNode {
	cn := &v2.CiliumNode{
		ObjectMeta: metav1.ObjectMeta{Name: node, Namespace: "default"},
		Spec: v2.NodeSpec{
			InstanceID: instanceID,
			ENI: eniTypes.ENISpec{
				InstanceType:        instanceType,
				FirstInterfaceIndex: &firstInterfaceIndex,
				AvailabilityZone:    az,
				VpcID:               vpcID,
				SecurityGroupTags:   sgTags,
			},
			IPAM: ipamTypes.IPAMSpec{
				Pool:        ipamTypes.AllocationMap{},
				PreAllocate: preAllocate,
				MinAllocate: minAllocate,
				MaxAllocate: maxAllocate,
			},
		},
		Status: v2.NodeStatus{
			IPAM: ipamTypes.IPAMStatus{
				Used: ipamTypes.AllocationMap{},
			},
		},
	}

	return cn
}

func updateCiliumNode(cn *v2.CiliumNode, available, used int) *v2.CiliumNode {
	cn.Spec.IPAM.Pool = ipamTypes.AllocationMap{}
	for i := 0; i < used; i++ {
		cn.Spec.IPAM.Pool[fmt.Sprintf("1.1.1.%d", i)] = ipamTypes.AllocationIP{Resource: "foo"}
	}

	cn.Status.IPAM.Used = ipamTypes.AllocationMap{}
	for ip, ipAllocation := range cn.Spec.IPAM.Pool {
		if used > 0 {
			delete(cn.Spec.IPAM.Pool, ip)
			cn.Status.IPAM.Used[ip] = ipAllocation
			used--
		}
	}

	return cn
}

func reachedAddressesNeeded(mngr *ipam.NodeManager, nodeName string, needed int) (success bool) {
	if node := mngr.Get(nodeName); node != nil {
		success = node.GetNeededAddresses() == needed
	}
	return
}

// TestNodeManagerDefaultAllocation tests allocation with default parameters
//
// - m5.large (3x ENIs, 2x10-2 IPs)
// - MinAllocate 0
// - MaxAllocate 0
// - PreAllocate 8
// - FirstInterfaceIndex 0
func (e *ENISuite) TestNodeManagerDefaultAllocation(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerDefaultAllocation-0", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node wait for IPs to become available
	cn := newCiliumNode("node1", "i-testNodeManagerDefaultAllocation-0", "m5.large", "us-west-1", "vpc-1", 0, 8, 0, 0)
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node1", 0) }, 5*time.Second), check.IsNil)

	node := mngr.Get("node1")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 8)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	// Use 7 out of 8 IPs
	mngr.Update(updateCiliumNode(cn, 8, 7))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node1", 0) }, 5*time.Second), check.IsNil)

	node = mngr.Get("node1")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 15)
	c.Assert(node.Stats().UsedIPs, check.Equals, 7)
}

// TestNodeManagerENIWithSGTags tests ENI allocation + association with a SG based on tags
//
// - m5.large (3x ENIs, 2x10-2 IPs)
// - MinAllocate 0
// - MaxAllocate 0
// - PreAllocate 8
// - FirstInterfaceIndex 0
func (e *ENISuite) TestNodeManagerENIWithSGTags(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerDefaultAllocation-0", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node wait for IPs to become available
	sgTags := map[string]string{
		"test-sg-1": "yes",
	}
	cn := newCiliumNodeWithSGTags("node1", "i-testNodeManagerDefaultAllocation-0", "m5.large", "us-west-1", "vpc-1", sgTags, 0, 8, 0, 0)
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node1", 0) }, 5*time.Second), check.IsNil)

	node := mngr.Get("node1")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 8)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	// Use 7 out of 8 IPs
	mngr.Update(updateCiliumNode(cn, 8, 7))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node1", 0) }, 5*time.Second), check.IsNil)

	node = mngr.Get("node1")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 15)
	c.Assert(node.Stats().UsedIPs, check.Equals, 7)

	// At this point we have 2 enis, make a local copy
	// and remove eth0 from the map
	eniNode, castOK := node.Ops().(*Node)
	c.Assert(castOK, check.Equals, true)
	eniNode.mutex.RLock()
	for id, eni := range eniNode.enis {
		if id != eniID1 {
			c.Assert(eni.SecurityGroups, checker.DeepEquals, []string{"sg-1"})
		}
	}
	eniNode.mutex.RUnlock()
}

// TestNodeManagerMinAllocate20 tests MinAllocate without PreAllocate
//
// - m5.4xlarge (8x ENIs, 7x30-7 IPs)
// - MinAllocate 10
// - MaxAllocate 0
// - PreAllocate -1
// - FirstInterfaceIndex 0
func (e *ENISuite) TestNodeManagerMinAllocate20(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerMinAllocate20-1", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node wait for IPs to become available
	cn := newCiliumNode("node2", "i-testNodeManagerMinAllocate20-1", "m5.4xlarge", "us-west-1", "vpc-1", 0, -1, 10, 0)
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)

	node := mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 10)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	mngr.Update(updateCiliumNode(cn, 10, 8))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)

	node = mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 10)
	c.Assert(node.Stats().UsedIPs, check.Equals, 8)

	// Change MinAllocate to 20
	cn = newCiliumNode("node2", "i-testNodeManagerMinAllocate20-1", "m5.4xlarge", "us-west-1", "vpc-1", 0, 0, 20, 0)
	mngr.Update(updateCiliumNode(cn, 20, 8))
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)

	node = mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 20)
	c.Assert(node.Stats().UsedIPs, check.Equals, 8)
}

// TestNodeManagerMinAllocateAndPreallocate tests MinAllocate in combination with PreAllocate
//
// - m3.large (3x ENIs, 2x10-2 IPs)
// - MinAllocate 10
// - MaxAllocate 0
// - PreAllocate 1
// - FirstInterfaceIndex 0
func (e *ENISuite) TestNodeManagerMinAllocateAndPreallocate(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerMinAllocateAndPreallocate-1", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node, wait for IPs to become available
	cn := newCiliumNode("node2", "i-testNodeManagerMinAllocateAndPreallocate-1", "m3.large", "us-west-1", "vpc-1", 0, 1, 10, 0)
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)

	node := mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 10)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	// Use 9 out of 10 IPs, no additional IPs should be allocated
	mngr.Update(updateCiliumNode(cn, 10, 9))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)
	node = mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 10)
	c.Assert(node.Stats().UsedIPs, check.Equals, 9)

	// Use 10 out of 10 IPs, PreAllocate 1 must kick in and allocate an additional IP
	mngr.Update(updateCiliumNode(cn, 10, 10))
	syncTime := instances.Resync(context.TODO())
	mngr.Resync(context.TODO(), syncTime)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)
	node = mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 11)
	c.Assert(node.Stats().UsedIPs, check.Equals, 10)

	// Release some IPs, no additional IPs should be allocated
	mngr.Update(updateCiliumNode(cn, 10, 8))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)
	node = mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 11)
	c.Assert(node.Stats().UsedIPs, check.Equals, 8)
}

// TestNodeManagerReleaseAddress tests PreAllocate, MinAllocate and MaxAboveWatermark
// when release excess IP is enabled
//
// - m4.large (4x ENIs, 3x15-3 IPs)
// - MinAllocate 10
// - MaxAllocate 0
// - PreAllocate 2
// - MaxAboveWatermark 3
// - FirstInterfaceIndex 0
func (e *ENISuite) TestNodeManagerReleaseAddress(c *check.C) {
	operatorOption.Config.ExcessIPReleaseDelay = 2
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerReleaseAddress-1", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, true)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node, wait for IPs to become available
	cn := newCiliumNode("node3", "i-testNodeManagerReleaseAddress-1", "m4.xlarge", "us-west-1", "vpc-1", 0, 2, 10, 0)
	cn.Spec.IPAM.MaxAboveWatermark = 3
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node3", 0) }, 5*time.Second), check.IsNil)

	// 10 min-allocate + 3 max-above-watermark => 13 IPs must become
	// available as 13 < 14 (interface limit)
	node := mngr.Get("node3")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 13)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	// Use 11 out of 13 IPs, no additional IPs should be allocated
	mngr.Update(updateCiliumNode(cn, 13, 11))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node3", 0) }, 5*time.Second), check.IsNil)
	node = mngr.Get("node3")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 13)
	c.Assert(node.Stats().UsedIPs, check.Equals, 11)

	// Use 13 out of 13 IPs, PreAllocate 2 + MaxAboveWatermark 3 must kick in
	// and allocate 5 additional IPs
	mngr.Update(updateCiliumNode(cn, 13, 13))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node3", 0) }, 5*time.Second), check.IsNil)
	node = mngr.Get("node3")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 18)
	c.Assert(node.Stats().UsedIPs, check.Equals, 13)

	// Reduce used IPs to 10, this leads to 8 excess IPs but release
	// occurs at interval based resync, so expect timeout at first
	mngr.Update(updateCiliumNode(cn, 18, 10))
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node3", 0) }, 2*time.Second), check.Not(check.IsNil))
	node = mngr.Get("node3")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 18)
	c.Assert(node.Stats().UsedIPs, check.Equals, 10)

	// Trigger resync manually, excess IPs should be released
	// 10 used + 3 pre-allocate + 2 max-above-watermark => 15
	node = mngr.Get("node3")
	eniNode, castOK := node.Ops().(*Node)
	c.Assert(castOK, check.Equals, true)
	obj := node.ResourceCopy()
	eniNode.mutex.RLock()
	obj.Status.ENI.ENIs = eniNode.enis
	eniNode.mutex.RUnlock()
	node.UpdatedResource(obj)

	// Excess timestamps should be registered after this
	syncTime := instances.Resync(context.TODO())
	mngr.Resync(context.TODO(), syncTime)

	// Acknowledge release IPs after 3 secs
	time.AfterFunc(3*time.Second, func() {
		// Excess delay duration should have elapsed by now, trigger resync again.
		// IPs should be marked as excess
		syncTime := instances.Resync(context.TODO())
		mngr.Resync(context.TODO(), syncTime)
		time.Sleep(1 * time.Second)
		node.PopulateIPReleaseStatus(obj)
		// Fake acknowledge IPs for release like agent would.
		testutils.FakeAcknowledgeReleaseIps(obj)
		node.UpdatedResource(obj)
		// Resync one more time to process acknowledgements.
		syncTime = instances.Resync(context.TODO())
		mngr.Resync(context.TODO(), syncTime)
	})

	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node3", 0) }, 5*time.Second), check.IsNil)
	node = mngr.Get("node3")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 15)
	c.Assert(node.Stats().UsedIPs, check.Equals, 10)
}

// TestNodeManagerExceedENICapacity tests exceeding ENI capacity
//
// - t2.xlarge (3x ENIs, 3x15-3 IPs)
// - MinAllocate 20
// - MaxAllocate 0
// - PreAllocate 8
// - FirstInterfaceIndex 0
func (e *ENISuite) TestNodeManagerExceedENICapacity(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerExceedENICapacity-1", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node, wait for IPs to become available
	cn := newCiliumNode("node2", "i-testNodeManagerExceedENICapacity-1", "t2.xlarge", "us-west-1", "vpc-1", 0, 8, 20, 0)
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)

	node := mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 20)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	// Use 40 out of 42 available IPs, we should reach 0 address needed once we
	// assigned the remaining 3 that the t2.xlarge instance type supports
	// (3x15 - 3 = 42 max)
	mngr.Update(updateCiliumNode(cn, 42, 40))
	syncTime := instances.Resync(context.TODO())
	mngr.Resync(context.TODO(), syncTime)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node2", 0) }, 5*time.Second), check.IsNil)

	node = mngr.Get("node2")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 42)
	c.Assert(node.Stats().UsedIPs, check.Equals, 40)
}

type nodeState struct {
	cn           *v2.CiliumNode
	name         string
	instanceName string
}

// TestNodeManagerManyNodes tests IP allocation of 100 nodes across 3 subnets
//
// - c3.xlarge (4x ENIs, 4x15-4 IPs)
// - MinAllocate 10
// - MaxAllocate 0
// - PreAllocate 1
// - FirstInterfaceIndex 1
func (e *ENISuite) TestNodeManagerManyNodes(c *check.C) {
	c.Skip("This test is flaky, see https://github.com/cilium/cilium/issues/11560")

	const (
		numNodes    = 100
		minAllocate = 10
	)

	subnets := []*ipamTypes.Subnet{
		{ID: "mgmt-1", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 100},
		{ID: "s-1", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 400},
		{ID: "s-2", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 400},
		{ID: "s-3", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 400},
	}

	ec2api := ec2mock.NewAPI(subnets, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instancesManager := NewInstancesManager(ec2api)
	mngr, err := ipam.NewNodeManager(instancesManager, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	state := make([]*nodeState, numNodes)

	for i := range state {
		eniID, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "mgmt-1", "desc", []string{"sg1", "sg2"})
		c.Assert(err, check.IsNil)
		_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, fmt.Sprintf("i-testNodeManagerManyNodes-%d", i), eniID)
		c.Assert(err, check.IsNil)
		instancesManager.Resync(context.TODO())
		s := &nodeState{name: fmt.Sprintf("node%d", i), instanceName: fmt.Sprintf("i-testNodeManagerManyNodes-%d", i)}
		s.cn = newCiliumNode(s.name, s.instanceName, "c3.xlarge", "us-west-1", "vpc-1", 1, 1, minAllocate, 0)
		state[i] = s
		mngr.Update(s.cn)
	}

	for _, s := range state {
		c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, s.name, 0) }, 5*time.Second), check.IsNil)

		node := mngr.Get(s.name)
		c.Assert(node, check.Not(check.IsNil))
		if node.Stats().AvailableIPs < minAllocate {
			c.Errorf("Node %s allocation shortage. expected at least: %d, allocated: %d", s.name, minAllocate, node.Stats().AvailableIPs)
			c.Fail()
		}
		c.Assert(node.Stats().UsedIPs, check.Equals, 0)
	}

	// The above check returns as soon as the address requirements are met.
	// The metrics may still be oudated, resync all nodes to update
	// metrics.
	mngr.Resync(context.TODO(), time.Now())

	c.Assert(metricsapi.Nodes("total"), check.Equals, numNodes)
	c.Assert(metricsapi.Nodes("in-deficit"), check.Equals, 0)
	c.Assert(metricsapi.Nodes("at-capacity"), check.Equals, 0)

	if allocated := metricsapi.AllocatedIPs("available"); allocated < numNodes*minAllocate {
		c.Errorf("IP %s allocation shortage. expected at least: %d, allocated: %d", numNodes*minAllocate, allocated)
		c.Fail()
	}
	c.Assert(metricsapi.AllocatedIPs("needed"), check.Equals, 0)
	c.Assert(metricsapi.AllocatedIPs("used"), check.Equals, 0)

	// All subnets must have been used for allocation
	for _, subnet := range subnets {
		c.Assert(metricsapi.AllocationAttempts("success", subnet.ID), check.Not(check.Equals), 0)
		c.Assert(metricsapi.IPAllocations(subnet.ID), check.Not(check.Equals), 0)
	}

	c.Assert(metricsapi.ResyncCount(), check.Not(check.Equals), 0)
	c.Assert(metricsapi.AvailableInterfaces(), check.Not(check.Equals), 0)
}

// TestNodeManagerInstanceNotRunning verifies that allocation correctly detects
// instances which are no longer running
//
// - FirstInterfaceIndex 1
func (e *ENISuite) TestNodeManagerInstanceNotRunning(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	metricsMock := metricsmock.NewMockMetrics()
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testNodeManagerInstanceNotRunning-0", eniID1)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	ec2api.SetMockError(ec2mock.AttachNetworkInterface, errors.New("foo is not 'running' foo"))
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	// Announce node, ENI attachement will fail
	cn := newCiliumNode("node1", "i-testNodeManagerInstanceNotRunning-0", "m4.large", "us-west-1", "vpc-1", 1, 8, 0, 0)
	mngr.Update(cn)

	// Wait for node to be declared notRunning
	c.Assert(testutils.WaitUntil(func() bool {
		if n := mngr.Get("node1"); n != nil {
			return !n.IsRunning()
		}
		return false
	}, 5*time.Second), check.IsNil)

	// Metric should not indicate failure
	c.Assert(metricsMock.AllocationAttempts("ENI attachment failed", testSubnet.ID), check.Equals, int64(0))

	node := mngr.Get("node1")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 0)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)
}

// TestInstanceBeenDeleted verifies that instance deletion is correctly detected
// and no further action is taken
//
// - m4.large (2x ENIs, 2x10-2 IPs)
// - FirstInterfaceIndex 0
func (e *ENISuite) TestInstanceBeenDeleted(c *check.C) {
	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	eniID1, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, "i-testInstanceBeenDeleted-0", eniID1)
	c.Assert(err, check.IsNil)
	eniID2, _, err := ec2api.CreateNetworkInterface(context.TODO(), 8, "s-1", "desc", []string{"sg1", "sg2"})
	c.Assert(err, check.IsNil)
	_, err = ec2api.AttachNetworkInterface(context.TODO(), 1, "i-testInstanceBeenDeleted-0", eniID2)
	c.Assert(err, check.IsNil)
	instances.Resync(context.TODO())
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	cn := newCiliumNode("node1", "i-testInstanceBeenDeleted-0", "m4.large", "us-west-1", "vpc-1", 0, 8, 0, 0)
	mngr.Update(cn)
	c.Assert(testutils.WaitUntil(func() bool { return reachedAddressesNeeded(mngr, "node1", 0) }, 5*time.Second), check.IsNil)

	node := mngr.Get("node1")
	c.Assert(node, check.Not(check.IsNil))
	c.Assert(node.Stats().AvailableIPs, check.Equals, 8)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)

	// Delete all enis attached to instance, this mocks the operation of
	// deleting the instance. The deletion should be detected.
	err = ec2api.DeleteNetworkInterface(context.TODO(), eniID1)
	c.Assert(err, check.IsNil)
	err = ec2api.DeleteNetworkInterface(context.TODO(), eniID2)
	c.Assert(err, check.IsNil)
	// Resync instances from mocked AWS
	instances.Resync(context.TODO())
	// Use 2 out of 9 IPs
	mngr.Update(updateCiliumNode(cn, 9, 2))

	// Instance deletion detected, no allocation happened despite of the IP deficit.
	c.Assert(node.Stats().AvailableIPs, check.Equals, 8)
	c.Assert(node.Stats().UsedIPs, check.Equals, 0)
	c.Assert(node.Stats().NeededIPs, check.Equals, 0)
	c.Assert(node.Stats().ExcessIPs, check.Equals, 0)
}

func benchmarkAllocWorker(c *check.C, workers int64, delay time.Duration, rateLimit float64, burst int) {
	testSubnet1 := &ipamTypes.Subnet{ID: "s-1", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 1000000}
	testSubnet2 := &ipamTypes.Subnet{ID: "s-2", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 1000000}
	testSubnet3 := &ipamTypes.Subnet{ID: "s-3", AvailabilityZone: "us-west-1", VirtualNetworkID: "vpc-1", AvailableAddresses: 1000000}

	ec2api := ec2mock.NewAPI([]*ipamTypes.Subnet{testSubnet1, testSubnet2, testSubnet3}, []*ipamTypes.VirtualNetwork{testVpc}, testSecurityGroups)
	ec2api.SetDelay(ec2mock.AllOperations, delay)
	ec2api.SetLimiter(rateLimit, burst)
	instances := NewInstancesManager(ec2api)
	c.Assert(instances, check.Not(check.IsNil))
	mngr, err := ipam.NewNodeManager(instances, k8sapi, metricsapi, 10, false)
	c.Assert(err, check.IsNil)
	c.Assert(mngr, check.Not(check.IsNil))

	state := make([]*nodeState, c.N)

	c.ResetTimer()
	for i := range state {
		eniID, _, err := ec2api.CreateNetworkInterface(context.TODO(), 0, "s-1", "desc", []string{"sg1", "sg2"})
		c.Assert(err, check.IsNil)
		_, err = ec2api.AttachNetworkInterface(context.TODO(), 0, fmt.Sprintf("i-benchmarkAllocWorker-%d", i), eniID)
		c.Assert(err, check.IsNil)
		instances.Resync(context.TODO())
		s := &nodeState{name: fmt.Sprintf("node%d", i), instanceName: fmt.Sprintf("i-benchmarkAllocWorker-%d", i)}
		s.cn = newCiliumNode(s.name, s.instanceName, "m4.large", "us-west-1", "vpc-1", 0, 1, 10, 0)
		state[i] = s
		mngr.Update(s.cn)
	}

restart:
	for _, s := range state {
		if !reachedAddressesNeeded(mngr, s.name, 0) {
			time.Sleep(5 * time.Millisecond)
			goto restart
		}
	}
	c.StopTimer()

}

func (e *ENISuite) BenchmarkAllocDelay20Worker1(c *check.C) {
	benchmarkAllocWorker(c, 1, 20*time.Millisecond, 100.0, 4)
}
func (e *ENISuite) BenchmarkAllocDelay20Worker10(c *check.C) {
	benchmarkAllocWorker(c, 10, 20*time.Millisecond, 100.0, 4)
}
func (e *ENISuite) BenchmarkAllocDelay20Worker50(c *check.C) {
	benchmarkAllocWorker(c, 50, 20*time.Millisecond, 100.0, 4)
}
func (e *ENISuite) BenchmarkAllocDelay50Worker1(c *check.C) {
	benchmarkAllocWorker(c, 1, 50*time.Millisecond, 100.0, 4)
}
func (e *ENISuite) BenchmarkAllocDelay50Worker10(c *check.C) {
	benchmarkAllocWorker(c, 10, 50*time.Millisecond, 100.0, 4)
}
func (e *ENISuite) BenchmarkAllocDelay50Worker50(c *check.C) {
	benchmarkAllocWorker(c, 50, 50*time.Millisecond, 100.0, 4)
}
