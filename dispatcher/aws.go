package dispatcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var s3alreadyUploadedSelf = map[string]bool{}
var errAmiNotFound = errors.New("AWS AMI Not Found")

func s3upload(guild *GuildStore, key string, reader io.Reader) error {
	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	cfg.Region = guild.Region
	client := s3.NewFromConfig(cfg)
	input := s3.PutObjectInput{Bucket: &guild.Bucket, Key: &key, Body: reader}
	_, err = client.PutObject(ctx, &input)
	return err
}

func s3uploadSelf(guild *GuildStore) error {
	if s3alreadyUploadedSelf[guild.Bucket] {
		return nil
	}
	var err error
	path, err := os.Executable()
	if err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	err = s3upload(guild, "narval", file)
	if err != nil {
		return err
	}
	s3alreadyUploadedSelf[guild.Bucket] = true
	return nil
}

func ec2makeServer(guild *GuildStore, variables map[string]string) error {
	ctx := context.TODO()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return err
	}
	cfg.Region = guild.Region
	client := ec2.NewFromConfig(cfg)

	// launch the instance :D
	imageId, err := ec2getBestAmi(ctx, client)
	if err != nil {
		return err
	}
	input := ec2.RunInstancesInput{
		ImageId:      imageId,
		InstanceType: types.InstanceTypeC5aLarge,
		UserData:     variablesToLauncherScript(variables),
	}
	_, err = client.RunInstances(ctx, &input)
	return err
}

func ec2getBestAmi(ctx context.Context, client *ec2.Client) (*string, error) {
	amiInput := ec2.DescribeImagesInput{Filters: []types.Filter{{
		Name:   aws.String("name"),
		Values: []string{"amzn2-ami-hvm-*-x86_64-gp2"},
	}}}
	amiOutput, err := client.DescribeImages(ctx, &amiInput)
	if err != nil {
		return nil, err
	}
	var mostRecentName, mostRecentDate *string
	mostRecentDate = aws.String("")
	for _, image := range amiOutput.Images {
		if *mostRecentDate < *image.CreationDate {
			mostRecentDate = image.CreationDate
			mostRecentName = image.Name
		}
	}
	if mostRecentName == nil {
		return nil, errAmiNotFound
	}
	return mostRecentName, nil
}

func variablesToLauncherScript(variables map[string]string) *string {
	var builder strings.Builder
	// aws requires the shebang line in the userdata to run
	builder.WriteString("#!/bin/bash\n")
	for name, value := range variables {
		value = strings.ReplaceAll(value, "'", "'\\''")
		builder.WriteString(fmt.Sprintf("export %s='%s'\n", name, value))
	}
	// start narval as common user
	builder.WriteString("aws s3 cp s3://$BUCKET/narval /opt/narval\n")
	builder.WriteString("chmod +x /opt/narval\n")
	builder.WriteString("sudo -u ec2-user -i /opt/narval\n")
	return aws.String(builder.String())
}
