package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/gookit/color"
)

func main() {
	var bucketName string
	flag.StringVar(&bucketName, "b", "", "the bucket name to check")

	var verbose bool
	flag.BoolVar(&verbose, "v", false, "see more info on attempts")

	flag.Parse()

	// Check if the bucketName was provided
	if bucketName == "" {
		fmt.Println("Error: the bucket name is required.")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.TODO()

	bucketRegion, err := getBucketRegion(bucketName)
	if err != nil {
		log.Fatalf("Unable to get the bucket region, %v", err)
	}

	if verbose {
		fmt.Printf("Bucket %s found in Region %s\n", bucketName, bucketRegion)
	}

	// Create a new client with the bucket's region for further operations
	bucketCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(string(bucketRegion)),
	)
	if err != nil {
		log.Fatalf("Unable to load SDK config for bucket region, %v", err)
	}

	// Create an S3 service client with the right region
	client := s3.NewFromConfig(bucketCfg)

	// Check the bucket ACL for open permissions
	bucketAclOutput, err := client.GetBucketAcl(ctx, &s3.GetBucketAclInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		if verbose {
			fmt.Printf("Failed to get ACL for bucket %s, %v\n", bucketName, err)
		}
	}

	if bucketAclOutput != nil {
		// Check if the bucket ACL includes permissions for public access
		for _, grant := range bucketAclOutput.Grants {
			if grant.Grantee.Type == types.TypeGroup && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				log.Printf("Bucket with open permissions found: %s", bucketName)
			}
		}

	}

	if verbose {
		fmt.Println("Iterating bucket contents...")
	}
	// List all objects in the bucket if bucket ACL check passes your criteria
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})

	// Iterate through the object list
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			log.Fatalf("Failed to get page, %v", err)
		}

		for _, object := range page.Contents {

			if verbose {
				fmt.Printf("Checking ACP on %s\n", *object.Key)
			}
			// Get the ACL for each object
			aclOutput, err := client.GetObjectAcl(ctx, &s3.GetObjectAclInput{
				Bucket: aws.String(bucketName),
				Key:    object.Key,
			})
			if err != nil {
				if verbose {
					log.Printf("Failed to get ACL for object %s, %v", *object.Key, err)
				}
				continue
			}

			// Check if the ACL includes permissions for overwrite by unauthorized users
			for _, grant := range aclOutput.Grants {
				if grant.Grantee.Type == types.TypeGroup && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
					if verbose {
						color.Green.Printf("Object with open permissions found: %s/%s\n", bucketName, *object.Key)
					} else {
						fmt.Printf("Object with open permissions found: %s/%s\n", bucketName, *object.Key)
					}

				}
			}
		}
	}
}

func getBucketRegion(bucketName string) (string, error) {

	url := fmt.Sprintf("https://%s.s3.amazonaws.com", bucketName)
	bucketRegion := ""

	// Perform a HEAD request to the bucket URL
	resp, err := http.Head(url)
	if err != nil {
		fmt.Printf("Error making HEAD request: %v\n", err)
		return "", err
	}
	defer resp.Body.Close()

	// Look for the x-amz-bucket-region header in the response
	region := resp.Header.Get("x-amz-bucket-region")
	if region == "" {
		fmt.Println("Bucket region not found in headers")
		return "", err
	} else {
		bucketRegion = region
	}
	return bucketRegion, nil
}
