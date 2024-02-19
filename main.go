package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

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

	var agressive bool
	flag.BoolVar(&agressive, "a", false, "be aggressive and attempt to write to the bucket / object policy")

	flag.Parse()

	// Check if the bucketName was provided
	if bucketName == "" {
		fmt.Println("Error: the bucket name is required.")
		flag.Usage()
		os.Exit(1)
	}

	ctx := context.TODO()

	/*
		gets and uses the correct bucket region
	*/
	bucketRegion, err := getBucketRegion(bucketName)
	if err != nil {
		log.Fatalf("Unable to get the bucket region, %v", err)
	}

	if verbose {
		fmt.Printf("Bucket %s found in Region %s\n", bucketName, bucketRegion)
	}

	bucketCfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(string(bucketRegion)),
	)
	if err != nil {
		log.Fatalf("Unable to load SDK config for bucket region, %v", err)
	}

	client := s3.NewFromConfig(bucketCfg)

	/*
		check the bucket ACL
	*/
	bucketAclOutput, err := client.GetBucketAcl(ctx, &s3.GetBucketAclInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		if verbose {
			fmt.Printf("Failed to get ACL for bucket %s, %v\n", bucketName, err)
		}
	}

	if bucketAclOutput != nil {
		for _, grant := range bucketAclOutput.Grants {
			if grant.Grantee.Type == types.TypeGroup && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
				fmt.Printf("Bucket with open permissions found: %s", bucketName)
			}
		}

	}

	/*
		attempts to upload a file to the bucket

	*/
	upload, err := testUpload(context.Background(), client, bucketName, "test-key.txt", strings.NewReader("This is a test"))
	if err != nil {
		if verbose {
			fmt.Printf("Failed to upload to the bucket %s\n", bucketName)
		}
	}
	if upload {
		if verbose {
			color.Green.Printf("Bucket %s allows uploads\n", bucketName)
		} else {
			fmt.Printf("Bucket %s allows uploads\n", bucketName)
		}
	}

	/*
		attempts to write to the ACP for the bucket
	*/
	if agressive {

		writeACP, err := putBucketACP(context.Background(), client, bucketName)
		if err != nil {
			if verbose {
				fmt.Printf("Failed to write the bucket ACP %s\n", bucketName)
			}
		}
		if writeACP {
			if verbose {
				color.Green.Printf("ACP for bucket %s changed!\n", bucketName)
			} else {
				fmt.Printf("ACP for bucket %s changed\n", bucketName)
			}
		}
	}
	/*
		iterates objects in the bucket and checks their ACLs
	*/
	if verbose {
		fmt.Println("Iterating bucket contents...")
	}

	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucketName),
	})

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
					fmt.Printf("Failed to get ACL for object %s, %v", *object.Key, err)
				}
				continue
			}

			// Check if the ACL includes permissions by unauthorized users
			for _, grant := range aclOutput.Grants {
				if grant.Grantee.Type == types.TypeGroup && *grant.Grantee.URI == "http://acs.amazonaws.com/groups/global/AllUsers" {
					if verbose {
						color.Green.Printf("Object with open permissions found: %s/%s\n", bucketName, *object.Key)
					} else {
						fmt.Printf("Object with open permissions found: %s/%s\n", bucketName, *object.Key)
					}

				}
			}

			if agressive {

				objectACP, err := putObjectACP(context.Background(), client, bucketName, *object.Key)
				if err != nil {
					if verbose {
						fmt.Printf("Failed to get update ACL for object %s\n", *object.Key)
					}
				}
				if objectACP {
					if verbose {
						color.Green.Printf("ACP for bucket object %s changed!\n", *object.Key)
					} else {
						fmt.Printf("ACP for bucket object %s changed\n", *object.Key)
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

func testUpload(ctx context.Context, client *s3.Client, bucket, key string, body *strings.Reader) (bool, error) {
	_, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   body,
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func putBucketACP(ctx context.Context, client *s3.Client, bucket string) (bool, error) {
	_, err := client.PutBucketAcl(ctx, &s3.PutBucketAclInput{
		Bucket:    aws.String(bucket),
		GrantRead: aws.String("uri=http://acs.amazonaws.com/groups/global/AuthenticatedUsers"),
	})
	if err != nil {
		return false, err
	}
	return true, nil
}

func putObjectACP(ctx context.Context, client *s3.Client, bucket, key string) (bool, error) {
	_, err := client.PutObjectAcl(ctx, &s3.PutObjectAclInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		ACL:    "public-read",
	})
	if err != nil {
		return false, err
	}
	return true, nil
}
