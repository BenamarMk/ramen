// SPDX-FileCopyrightText: The RamenDR authors
// SPDX-License-Identifier: Apache-2.0

package kubeobjects

import "strconv"

// CapturePathNamesAndNamePrefix returns, for a single capture of a single VRG:
//   - pathName: the S3 key prefix holding everything that capture wrote
//   - capturePathName: the prefix the capture's protect requests live under
//   - namePrefix: the prefix shared by every request name in the capture
//
// Together these define the contract between the capture side and every
// consumer of a capture. Callers must derive them here rather than
// reimplementing them: two copies that drift would not fail loudly, they would
// look in the wrong place and quietly restore nothing.
//
// vrgS3PathPrefix is the VRG's S3 root, owned by the controllers package.
// vrgNamespaceName and vrgName are taken separately because request names join
// them with "--" where paths join them with "/".
func CapturePathNamesAndNamePrefix(
	vrgS3PathPrefix, vrgNamespaceName, vrgName string,
	captureNumber int64,
	kubeObjects RequestsManager,
) (string, string, string) {
	const numberBase = 10

	number := strconv.FormatInt(captureNumber, numberBase)
	pathName := vrgS3PathPrefix + "kube-objects/" + number + "/"

	return pathName,
		pathName + kubeObjects.ProtectsPath(),
		// TODO fix: may exceed name capacity
		vrgNamespaceName + "--" + vrgName + "--" + number
}

// CaptureName returns the name of one request within a capture, given the
// namePrefix returned by CapturePathNamesAndNamePrefix.
func CaptureName(prefix, groupName, s3ProfileName string) string {
	return prefix + "--" + groupName + "--" + s3ProfileName
}
