package chaincode

import (
	// "bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"strconv"

	// "time"
	// "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/uuid"
	"github.com/hyperledger/fabric-chaincode-go/pkg/statebased"
	"github.com/hyperledger/fabric-chaincode-go/shim"
	"github.com/hyperledger/fabric-contract-api-go/contractapi"
)

// Asset describes main asset details that are visible to all organizations
type Asset struct {
	Type     string  `json:"objectType"` //Type is used to distinguish the various types of objects in state database
	AssetKey string  `json:"assetKey"`
	AssetID  string  `json:"assetID"`
	PrevID   string  `json:"prevAssetID"`
	Asset    string  `json:"asset"`
	Qty      float32 `json:"qty"`
	Owner    string  `json:"owner"`
	Active   string  `json:"active"`
	Version  int     `json:"version"`
}

type BuyerOrg struct {
	BuyerOrgID string `json:"buyerOrgID,omitempty"`
}

// CreateAsset creates a new asset by placing the main asset details in the assetCollection
// that can be read by both organizations. The appraisal value is stored in the owners org specific collection.
func (s *SmartContract) CreateAsset(ctx contractapi.TransactionContextInterface, assetJSON []byte) ([]byte, error) {

	var assetInput Asset
	err := json.Unmarshal(assetJSON, &assetInput)
	if err != nil {
		return []byte("Error"), fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	if len(assetInput.Type) == 0 {
		return []byte("Error"), fmt.Errorf("objectType field must be a non-empty string")
	}
	// if len(assetInput.AssetID) == 0 {
	// 	return fmt.Errorf("assetID field must be a non-empty string")
	// }
	if len(assetInput.Asset) == 0 {
		return []byte("Error"), fmt.Errorf("asset field must be a non-empty string")
	}
	if assetInput.Qty <= 0.0 {
		return []byte("Error"), fmt.Errorf("qty field must be a positive integer")
	}
	if len(assetInput.Owner) == 0 {
		return []byte("Error"), fmt.Errorf("owner field must be a non-empty string")
	}

	// Get the clientOrgId from the input, will be used for implicit collection, owner, and state-based endorsement policy
	clientOrgID, err := getClientOrgID(ctx)
	if err != nil {
		return []byte("Error"), err
	}

	// In this scenario, client is only authorized to read/write private data from its own peer, therefore verify client org id matches peer org id.
	err = verifyClientOrgMatchesPeerOrg(clientOrgID)
	if err != nil {
		return []byte("Error"), err
	}

	// Persist private immutable asset properties to owner's private data collection
	ownerCollection := buildCollectionName(clientOrgID)

	// Check if asset already exists
	// assetAsBytes, err := ctx.GetStub().GetPrivateData(ownerCollection, assetInput.AssetKey)
	// if err != nil {
	// 	return []byte("Error"), fmt.Errorf("failed to get asset: %v", err)
	// } else if assetAsBytes != nil {
	// 	fmt.Println("Asset already exists: " + assetInput.AssetID)
	// 	return []byte("Error"), fmt.Errorf("this asset already exists: " + assetInput.AssetID)
	// }

	//Generating new uuid for assetID
	uuidAsset := uuid.New()
	timestamp, err := getTimestamp()
	if err != nil {
		return []byte("Error"), fmt.Errorf("failed to get timestamp: %v", err)
	}

	newAssetKey, err := ctx.GetStub().CreateCompositeKey(assetInput.Type, []string{uuidAsset.String(), "-", timestamp})
	if err != nil {
		return []byte("Error"), fmt.Errorf("failed to create composite key for new asset: %v", err)
	}

	// Make submitting client the owner
	asset := Asset{
		Type:     assetInput.Type,
		AssetKey: newAssetKey,
		AssetID:  uuidAsset.String(),
		PrevID:   assetInput.PrevID,
		Asset:    assetInput.Asset,
		Qty:      assetInput.Qty,
		Owner:    assetInput.Owner,
		Active:   "A",
		Version:  0,
	}
	assetJSONasBytes, err := json.Marshal(asset)
	if err != nil {
		return []byte("Error"), fmt.Errorf("failed to marshal asset into JSON: %v", err)
	}

	// Save asset to private data collection
	// Typical logger, logs to stdout/file in the fabric managed docker container, running this chaincode
	// Look for container name like dev-peer0.org1.example.com-{chaincodename_version}-xyz
	log.Printf("CreateAsset Put: collection %v, ID %v, owner %v", ownerCollection, newAssetKey, clientOrgID)

	err = ctx.GetStub().PutPrivateData(ownerCollection, newAssetKey, assetJSONasBytes)
	if err != nil {
		return []byte("Error"), fmt.Errorf("failed to put asset into private data collecton: %v", err)
	}

	// Set the endorsement policy such that an owner org peer is required to endorse future updates.
	// In practice, consider additional endorsers such as a trusted third party to further secure transfers.
	endorsingOrgs := []string{clientOrgID}
	err = setAssetPrivateStateBasedEndorsement(ctx, ownerCollection, newAssetKey, endorsingOrgs)
	if err != nil {
		return []byte("Error"), fmt.Errorf("failed setting state based endorsement for buyer and seller: %v", err)
	}

	return assetJSONasBytes, nil
}

// TransferAsset checks transfer conditions and then transfers asset state to buyer.
// TransferAsset can only be called by current owner
func (s *SmartContract) TransferAsset(ctx contractapi.TransactionContextInterface, assetTransferJSON []byte, buyerOrg string) error {

	// type AssetTransfer struct {
	// 	AssetKey string `json:"assetKey"`
	// 	BuyerOrgID string `json:"buyerOrgID"`
	// }
	log.Printf("TransferAsset %v, Asset %v, new buyer %v", assetTransferJSON, buyerOrg)

	var assetTransferInput Asset
	err := json.Unmarshal(assetTransferJSON, &assetTransferInput)
	if err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %v", err)
	}

	// if len(assetTransferInput.OriginalAsset) == 0 {
	// 	return fmt.Errorf("assetID field must be a non-empty string")
	// }

	// var buyerOrgInput BuyerOrg
	// err = json.Unmarshal(buyerOrgJSON, &buyerOrgInput)
	// if err != nil {
	// 	return fmt.Errorf("failed to unmarshal JSON: %v", err)
	// }

	if len(buyerOrg) == 0 {
		return fmt.Errorf("BuyerOrgID field must be non-empty string")
	}

	clientOrgID, err := getClientOrgID(ctx)
	if err != nil {
		return err
	}

	log.Printf("clientOrgID: %v %v", clientOrgID, buyerOrg)
	transferAssetState(ctx, &assetTransferInput, clientOrgID, buyerOrg)

	return nil
}

func transferAssetState(ctx contractapi.TransactionContextInterface, asset *Asset, clientOrgID string, buyerOrgID string) error {

	// New owner asset creation
	newOwnerCollection := buildCollectionName(buyerOrgID)

	// Update ownership in public state
	asset.Owner = buyerOrgID

	// asset.AssetKey = newAssetKey
	updatedAsset, err := json.Marshal(asset)
	if err != nil {
		return err
	}

	log.Printf("CreateAsset Put: collection %v, ID %v, owner %v", newOwnerCollection, asset.AssetID, asset.AssetKey, buyerOrgID)

	err = ctx.GetStub().PutPrivateData(newOwnerCollection, asset.AssetKey, updatedAsset)
	if err != nil {
		return fmt.Errorf("failed to put asset into private data collecton: %v", err)
	}

	//Changes the endorsement policy to the new owner org
	endorsingOrgs := []string{buyerOrgID}
	err = setAssetPrivateStateBasedEndorsement(ctx, newOwnerCollection, asset.AssetKey, endorsingOrgs)
	if err != nil {
		return fmt.Errorf("failed setting state based endorsement for new owner: %v", err)
	}

	return nil
}

// getCollectionName is an internal helper function to get collection of submitting client identity.
func getCollectionName(ctx contractapi.TransactionContextInterface) (string, error) {

	// Get the MSP ID of submitting client identity
	clientMSPID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed to get verified MSPID: %v", err)
	}

	// Create the collection name
	orgCollection := clientMSPID + "PrivateCollection"

	return orgCollection, nil
}

// verifyClientOrgMatchesPeerOrg checks that the client is from the same org as the peer
func verifyClientOrgMatchesPeerOrg(clientOrgID string) error {
	peerOrgID, err := shim.GetMSPID()
	if err != nil {
		return fmt.Errorf("failed getting peer's orgID: %v", err)
	}

	if clientOrgID != peerOrgID {
		return fmt.Errorf("client from org %s is not authorized to read or write private data from an org %s peer",
			clientOrgID,
			peerOrgID,
		)
	}

	return nil
}

func submittingClientIdentity(ctx contractapi.TransactionContextInterface) (string, error) {
	b64ID, err := ctx.GetClientIdentity().GetID()
	if err != nil {
		return "", fmt.Errorf("Failed to read clientID: %v", err)
	}
	decodeID, err := base64.StdEncoding.DecodeString(b64ID)
	if err != nil {
		return "", fmt.Errorf("failed to base64 decode clientID: %v", err)
	}
	return string(decodeID), nil
}

// setAssetStateBasedEndorsement adds an endorsement policy to an asset so that the passed orgs need to agree upon transfer
func setAssetStateBasedEndorsement(ctx contractapi.TransactionContextInterface, assetID string, orgsToEndorse []string) error {
	endorsementPolicy, err := statebased.NewStateEP(nil)
	if err != nil {
		return err
	}
	err = endorsementPolicy.AddOrgs(statebased.RoleTypePeer, orgsToEndorse...)
	if err != nil {
		return fmt.Errorf("failed to add org to endorsement policy: %v", err)
	}
	policy, err := endorsementPolicy.Policy()
	if err != nil {
		return fmt.Errorf("failed to create endorsement policy bytes from org: %v", err)
	}
	err = ctx.GetStub().SetStateValidationParameter(assetID, policy)
	if err != nil {
		return fmt.Errorf("failed to set validation parameter on asset: %v", err)
	}

	return nil
}

// setAssetPrivateStateBasedEndorsement adds an endorsement policy to an asset so that the passed orgs need to agree upon transfer
func setAssetPrivateStateBasedEndorsement(ctx contractapi.TransactionContextInterface, collection string, assetID string, orgsToEndorse []string) error {
	endorsementPolicy, err := statebased.NewStateEP(nil)
	if err != nil {
		return err
	}
	err = endorsementPolicy.AddOrgs(statebased.RoleTypePeer, orgsToEndorse...)
	if err != nil {
		return fmt.Errorf("failed to add org to endorsement policy: %v", err)
	}
	policy, err := endorsementPolicy.Policy()
	if err != nil {
		return fmt.Errorf("failed to create endorsement policy bytes from org: %v", err)
	}
	err = ctx.GetStub().SetPrivateDataValidationParameter(collection, assetID, policy)
	if err != nil {
		return fmt.Errorf("failed to set validation parameter on asset: %v", err)
	}

	orgs := endorsementPolicy.ListOrgs()
	log.Printf("endorsing orgs : %v", orgs)

	return nil
}

// getClientImplicitCollectionNameAndVerifyClientOrg gets the implicit collection for the client and checks that the client is from the same org as the peer
func getClientImplicitCollectionNameAndVerifyClientOrg(ctx contractapi.TransactionContextInterface) (string, error) {
	clientOrgID, err := getClientOrgID(ctx)
	if err != nil {
		return "", err
	}

	err = verifyClientOrgMatchesPeerOrg(clientOrgID)
	if err != nil {
		return "", err
	}

	return buildCollectionName(clientOrgID), nil
}

// getClientOrgID gets the client org ID.
func getClientOrgID(ctx contractapi.TransactionContextInterface) (string, error) {
	clientOrgID, err := ctx.GetClientIdentity().GetMSPID()
	if err != nil {
		return "", fmt.Errorf("failed getting client's orgID: %v", err)
	}

	return clientOrgID, nil
}

// buildCollectionName returns the implicit collection name for an org
func buildCollectionName(clientOrgID string) string {
	return fmt.Sprintf("_implicit_org_%s", clientOrgID)
}

func getTimestamp() (string, error) {
	// txTimestamp, err := ctx.GetStub().GetTxTimestamp()
	// if err != nil {
	// 	return "", fmt.Errorf("failed to create timestamp for receipt: %v", err)
	// }

	// timestamp, err := ptypes.Timestamp(txTimestamp)
	// if err != nil {
	// 	return "", err
	// }

	// now := time.Now().UTC()
	// secs := now.Unix()
	// nanos := int32(now.UnixNano() - (secs * 1000000000))
	// timestampStr := &(timestamp.Timestamp{secs,nanos})

	value := rand.Int()

	return strconv.Itoa(value), nil
}
