/*
 * Copyright 2017 Google Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package nozzle

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/cloudfoundry-community/stackdriver-tools/src/stackdriver-nozzle/cloudfoundry"
	"github.com/cloudfoundry/sonde-go/events"
)

type LabelMaker interface {
	Build(*events.Envelope) map[string]string
}

func NewLabelMaker(appInfoRepository cloudfoundry.AppInfoRepository) LabelMaker {
	return &labelMaker{appInfoRepository: appInfoRepository}
}

type labelMaker struct {
	appInfoRepository cloudfoundry.AppInfoRepository
}

type labelMap map[string]string

func (labels labelMap) setIfNotEmpty(key, value string) {
	if value != "" {
		labels[key] = value
	}
}

func (labels labelMap) setValueOrUnknown(key, value string) {
	if value == "" {
		labels[key] = "unknown_" + key
	} else {
		labels[key] = value
	}
}

func (labels labelMap) path(keys ...string) string {
	var b bytes.Buffer
	for _, k := range keys {
		if _, ok := labels[k]; !ok {
			continue
		}
		b.WriteByte('/')
		b.WriteString(labels[k])
	}
	return b.String()
}

// Build extracts metric metadata from the event envelope and event contained
// within, and constructs a set of StackDriver (SD) metric labels from them.
//
// Since SD only allows 10 custom labels per metric, we collapse most metadata
// into two "paths" one representing the metric origin and one representing the
// serving application. We maintain vm and application instance indexes as
// separate labels so that it is easy to aggregate across multiple instances.
// We maintain "deployment" as a separate label to facilitate monitoring
// multple PCF instances within a GCP project, though this does require
// users to name their PCF deployment on bosh something other than "cf".:
func (lm *labelMaker) Build(envelope *events.Envelope) map[string]string {
	labels := labelMap{}
	labels.setIfNotEmpty("deployment", envelope.GetDeployment())
	labels.setIfNotEmpty("originPath", lm.getOriginPath(envelope))
	labels.setIfNotEmpty("index", envelope.GetIndex())
	labels.setIfNotEmpty("applicationPath", lm.getApplicationPath(envelope))
	labels.setIfNotEmpty("instanceIndex", getInstanceIndex(envelope))

	// Copy over tags from the envelope into labels.
	for k, v := range envelope.GetTags() {
		labels[k] = v
	}

	return labels
}

// getOriginPath returns a path that uniquely identifies a metric origin.
// The path hierarchy is /job/origin, e.g.
//     /diego_brain/tps_listener
func (lm *labelMaker) getOriginPath(envelope *events.Envelope) string {
	labels := labelMap{}
	labels.setValueOrUnknown("job", envelope.GetJob())
	labels.setValueOrUnknown("origin", envelope.GetOrigin())
	return labels.path("job", "origin")
}

// getApplicationPath returns a path that uniquely identifies a
// collection of instances of a given application running in an org + space.
// The path heirarchy is /deployment/org/space/application, e.g.
//     /system/autoscaling/autoscale
func (lm *labelMaker) getApplicationPath(envelope *events.Envelope) string {
	appID := getApplicationId(envelope)
	if appID == "" {
		return ""
	}
	app := lm.appInfoRepository.GetAppInfo(appID)
	if app.AppName == "" {
		return ""
	}

	labels := labelMap{}
	labels.setValueOrUnknown("org", app.OrgName)
	labels.setValueOrUnknown("space", app.SpaceName)
	labels.setValueOrUnknown("application", app.AppName)

	return labels.path("org", "space", "application")
}

// getApplicationId extracts the application UUID from the event contained
// within the envelope, for those events that have application IDs.
func getApplicationId(envelope *events.Envelope) string {
	switch envelope.GetEventType() {
	case events.Envelope_HttpStartStop:
		return formatUUID(envelope.GetHttpStartStop().GetApplicationId())
	case events.Envelope_LogMessage:
		return envelope.GetLogMessage().GetAppId()
	case events.Envelope_ContainerMetric:
		return envelope.GetContainerMetric().GetApplicationId()
	}
	return ""
}

// getInstanceIndex extracts the instance index or UUID from the event
// contained within the envelope, for those events that have instance IDs.
func getInstanceIndex(envelope *events.Envelope) string {
	switch envelope.GetEventType() {
	case events.Envelope_HttpStartStop:
		hss := envelope.GetHttpStartStop()
		if hss != nil && hss.InstanceIndex != nil {
			return fmt.Sprintf("%d", hss.GetInstanceIndex())
		}
		// Sometimes InstanceIndex is not set but InstanceId is; fall back.
		return hss.GetInstanceId()
	case events.Envelope_LogMessage:
		return envelope.GetLogMessage().GetSourceInstance()
	case events.Envelope_ContainerMetric:
		return fmt.Sprintf("%d", envelope.GetContainerMetric().GetInstanceIndex())
	}
	return ""
}

func formatUUID(uuid *events.UUID) string {
	if uuid == nil {
		return ""
	}
	var uuidBytes [16]byte
	binary.LittleEndian.PutUint64(uuidBytes[:8], uuid.GetLow())
	binary.LittleEndian.PutUint64(uuidBytes[8:], uuid.GetHigh())
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuidBytes[0:4], uuidBytes[4:6], uuidBytes[6:8], uuidBytes[8:10], uuidBytes[10:])
}
