// Copyright (c) 2022-2022 北京渠成软件有限公司(Beijing Qucheng Software Co., Ltd. www.qucheng.com) All rights reserved.
// Use of this source code is covered by the following dual licenses:
// (1) Z PUBLIC LICENSE 1.2 (ZPL 1.2)
// (2) Affero General Public License 3.0 (AGPL 3.0)
// license that can be found in the LICENSE file.

package util

// AddLabel returns a map with the given key and value added to the given map.
func AddLabel(labels map[string]string, labelKey, labelValue string) map[string]string {
	if labelKey == "" {
		// Don't need to add a label.
		return labels
	}
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[labelKey] = labelValue
	return labels
}

// CheckLabel returns key exist stauts.
func CheckLabel(labels map[string]string, labelKey string) bool {
	if labelKey == "" {
		// Don't need to add a label.
		return true
	}
	if labels == nil {
		labels = make(map[string]string)
	}

	if _, ok := labels[labelKey]; ok {
		return true
	}
	return false
}

// GetLabelValue returns key exist stauts.
func GetLabelValue(labels map[string]string, labelKey string) string {
	if labelKey == "" {
		// Don't need to add a label.
		return ""
	}
	if labels == nil {
		labels = make(map[string]string)
	}

	if v, ok := labels[labelKey]; ok {
		return v
	}
	return ""
}
