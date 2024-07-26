// Code generated by counterfeiter. DO NOT EDIT.
package fake

import (
	"context"
	"sync"

	"code.cloudfoundry.org/korifi/api/authorization"
	"code.cloudfoundry.org/korifi/api/handlers"
	"code.cloudfoundry.org/korifi/api/repositories"
)

type PodRepository struct {
	DeletePodStub        func(context.Context, authorization.Info, string, repositories.ProcessRecord, string) error
	deletePodMutex       sync.RWMutex
	deletePodArgsForCall []struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
		arg4 repositories.ProcessRecord
		arg5 string
	}
	deletePodReturns struct {
		result1 error
	}
	deletePodReturnsOnCall map[int]struct {
		result1 error
	}
	invocations      map[string][][]interface{}
	invocationsMutex sync.RWMutex
}

func (fake *PodRepository) DeletePod(arg1 context.Context, arg2 authorization.Info, arg3 string, arg4 repositories.ProcessRecord, arg5 string) error {
	fake.deletePodMutex.Lock()
	ret, specificReturn := fake.deletePodReturnsOnCall[len(fake.deletePodArgsForCall)]
	fake.deletePodArgsForCall = append(fake.deletePodArgsForCall, struct {
		arg1 context.Context
		arg2 authorization.Info
		arg3 string
		arg4 repositories.ProcessRecord
		arg5 string
	}{arg1, arg2, arg3, arg4, arg5})
	stub := fake.DeletePodStub
	fakeReturns := fake.deletePodReturns
	fake.recordInvocation("DeletePod", []interface{}{arg1, arg2, arg3, arg4, arg5})
	fake.deletePodMutex.Unlock()
	if stub != nil {
		return stub(arg1, arg2, arg3, arg4, arg5)
	}
	if specificReturn {
		return ret.result1
	}
	return fakeReturns.result1
}

func (fake *PodRepository) DeletePodCallCount() int {
	fake.deletePodMutex.RLock()
	defer fake.deletePodMutex.RUnlock()
	return len(fake.deletePodArgsForCall)
}

func (fake *PodRepository) DeletePodCalls(stub func(context.Context, authorization.Info, string, repositories.ProcessRecord, string) error) {
	fake.deletePodMutex.Lock()
	defer fake.deletePodMutex.Unlock()
	fake.DeletePodStub = stub
}

func (fake *PodRepository) DeletePodArgsForCall(i int) (context.Context, authorization.Info, string, repositories.ProcessRecord, string) {
	fake.deletePodMutex.RLock()
	defer fake.deletePodMutex.RUnlock()
	argsForCall := fake.deletePodArgsForCall[i]
	return argsForCall.arg1, argsForCall.arg2, argsForCall.arg3, argsForCall.arg4, argsForCall.arg5
}

func (fake *PodRepository) DeletePodReturns(result1 error) {
	fake.deletePodMutex.Lock()
	defer fake.deletePodMutex.Unlock()
	fake.DeletePodStub = nil
	fake.deletePodReturns = struct {
		result1 error
	}{result1}
}

func (fake *PodRepository) DeletePodReturnsOnCall(i int, result1 error) {
	fake.deletePodMutex.Lock()
	defer fake.deletePodMutex.Unlock()
	fake.DeletePodStub = nil
	if fake.deletePodReturnsOnCall == nil {
		fake.deletePodReturnsOnCall = make(map[int]struct {
			result1 error
		})
	}
	fake.deletePodReturnsOnCall[i] = struct {
		result1 error
	}{result1}
}

func (fake *PodRepository) Invocations() map[string][][]interface{} {
	fake.invocationsMutex.RLock()
	defer fake.invocationsMutex.RUnlock()
	fake.deletePodMutex.RLock()
	defer fake.deletePodMutex.RUnlock()
	copiedInvocations := map[string][][]interface{}{}
	for key, value := range fake.invocations {
		copiedInvocations[key] = value
	}
	return copiedInvocations
}

func (fake *PodRepository) recordInvocation(key string, args []interface{}) {
	fake.invocationsMutex.Lock()
	defer fake.invocationsMutex.Unlock()
	if fake.invocations == nil {
		fake.invocations = map[string][][]interface{}{}
	}
	if fake.invocations[key] == nil {
		fake.invocations[key] = [][]interface{}{}
	}
	fake.invocations[key] = append(fake.invocations[key], args)
}

var _ handlers.PodRepository = new(PodRepository)