/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package chaincode

import (

	"fmt"
	"log"
	// "github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

const assetCollection = "assetCollection"
const transferAgreementObjectType = "transferAgreement"

// SmartContract of this fabric sample
type SmartContract struct {
	contractapi.Contract
}
// CreateAsset creates a new asset by placing the main asset details in the assetCollection
// that can be read by both organizations. The appraisal value is stored in the owners org specific collection.
func (s *SmartContract) IssueAsset(ctx contractapi.TransactionContextInterface) error {

	// Get new asset from transient map
	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("error getting transient: %v", err)
	}

	// Asset properties are private, therefore they get passed in transient field, instead of func args
	transientAssetJSON, ok := transientMap["asset_properties"]
	if !ok {
		//log error to stdout
		return fmt.Errorf("asset not found in the transient map input")
	}

	asset, err := s.CreateAsset(ctx, transientAssetJSON)
	if err != nil {
		return fmt.Errorf("failed to create asset", err)
	}

	log.Printf("CreateAsset successfully : %v", string(asset))

	return nil
}

func (s *SmartContract) TransferAssetMain(ctx contractapi.TransactionContextInterface) error {

	transientMap, err := ctx.GetStub().GetTransient()
	if err != nil {
		return fmt.Errorf("Error getting transient: %v", err)
	}

	// Asset properties are private, therefore they get passed in transient field
	transientTransferJSON, ok := transientMap["asset_transfer"]
	if !ok {
		return fmt.Errorf("asset to transfer not found in the transient map")
	}

	// // Asset properties are private, therefore they get passed in transient field
	// transientBuyerOrg, ok := transientMap["buyer_org"]
	// if !ok {
	// 	return fmt.Errorf("buyer org not found in the transient map")
	// }

	transientBuyerOrg := "Org2MSP";
	
	err = s.TransferAsset(ctx, []byte(transientTransferJSON), string(transientBuyerOrg))
	if err != nil {
		return fmt.Errorf("Error tranfer the asset: %v", err)
	}

	log.Printf("TransferAssetMain successfully")

	return nil
}

