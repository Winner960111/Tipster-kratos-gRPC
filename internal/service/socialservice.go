package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	pb "Tipster/api/Tipster"
	"Tipster/internal/data"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type SocialServiceService struct {
	pb.UnimplementedSocialServiceServer
	MongoClient *data.MongoDB
}

func NewSocialServiceService(mongoDB *data.MongoDB) *SocialServiceService {
	return &SocialServiceService{MongoClient: mongoDB}
}

func (s *SocialServiceService) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	currentTime := time.Now().UTC()
	collection := s.MongoClient.Database.Collection("users")
	// Check if email already exists
	emailFilter := bson.M{"email": req.Email}
	existingUser := collection.FindOne(ctx, emailFilter)
	if existingUser.Err() != mongo.ErrNoDocuments {
		return &pb.CreateUserResponse{
			Code: "COMM0400",
			Msg:  "Email already exists",
		}, nil
	}

	userDoc := map[string]interface{}{
		"username":  req.UserName,
		"password":  req.Password,
		"email":     req.Email,
		"tags":      req.Tags,
		"following": []primitive.ObjectID{},
		"followers": []primitive.ObjectID{},
		"createdAt": currentTime,
		"updatedAt": currentTime,
	}

	result, err := collection.InsertOne(ctx, userDoc)
	if err != nil {
		log.Fatal("MongoDB Insertion Error:", err)
	}
	fmt.Println("Successfully created user")

	return &pb.CreateUserResponse{
		Code: "COMM0000",
		Msg:  "User created successfully",
		Data: &pb.CreateUserResponse_UserData{
			UserId:    result.InsertedID.(primitive.ObjectID).Hex(),
			UserName:  req.UserName,
			Email:     req.Email,
			Tags:      req.Tags,
			CreatedAt: timestamppb.New(currentTime),
			UpdatedAt: timestamppb.New(currentTime),
		},
	}, nil
}
func (s *SocialServiceService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	collection := s.MongoClient.Database.Collection("users")

	objID, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.GetUserResponse{
			Code: "COMM0101",
			Msg:  "Invalid user ID format",
		}, nil
	}

	filter := bson.M{"_id": objID}
	// Use a struct instead of map to avoid type assertion issues
	var user struct {
		ID         primitive.ObjectID   `bson:"_id"`
		UserName   string               `bson:"username"`
		Email      string               `bson:"email"`
		Tags       []string             `bson:"tags"`
		CreatedAt  primitive.DateTime   `bson:"createdAt"`
		UpdatedAt  primitive.DateTime   `bson:"updatedAt"`
		Followers  []primitive.ObjectID `bson:"followers"`
		Followings []primitive.ObjectID `bson:"following"`
	}
	err = collection.FindOne(ctx, filter).Decode(&user)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Errorf(codes.NotFound, "User not found")
		}
		return nil, status.Errorf(codes.Internal, "Error fetching user: %v", err)
	}

	// Get follower details
	var followerDetails []*pb.UserDetail
	if len(user.Followers) > 0 {
		followerFilter := bson.M{"_id": bson.M{"$in": user.Followers}}
		cursor, err := collection.Find(ctx, followerFilter)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error fetching followers: %v", err)
		}
		defer cursor.Close(ctx)

		for cursor.Next(ctx) {
			var follower struct {
				ID       primitive.ObjectID `bson:"_id"`
				UserName string             `bson:"username"`
			}
			if err := cursor.Decode(&follower); err == nil {
				followerDetails = append(followerDetails, &pb.UserDetail{
					Id:       strings.TrimSpace(follower.ID.Hex()), // Trim spaces
					UserName: strings.TrimSpace(follower.UserName),
				})
			}
		}
	}

	// Get following details
	var followingDetails []*pb.UserDetail
	if len(user.Followings) > 0 {
		followingFilter := bson.M{"_id": bson.M{"$in": user.Followings}}
		cursor, err := collection.Find(ctx, followingFilter)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error fetching followings: %v", err)
		}
		defer cursor.Close(ctx)

		for cursor.Next(ctx) {
			var following struct {
				ID       primitive.ObjectID `bson:"_id"`
				UserName string             `bson:"username"`
			}
			if err := cursor.Decode(&following); err == nil {
				// 🔹 Make sure we return a valid struct, not a pointer
				userDetail := pb.UserDetail{
					Id:       strings.TrimSpace(following.ID.Hex()),
					UserName: strings.TrimSpace(following.UserName),
				}
				followingDetails = append(followingDetails, &userDetail)
			}
		}
	}
	return &pb.GetUserResponse{
		Code: "COMM0000",
		Msg:  "User found successfully",
		Data: &pb.CreateUserResponse_UserData{
			UserId:     user.ID.Hex(),
			UserName:   user.UserName,
			Email:      user.Email,
			Tags:       user.Tags,
			CreatedAt:  timestamppb.New(user.CreatedAt.Time()),
			UpdatedAt:  timestamppb.New(user.UpdatedAt.Time()),
			Followers:  followerDetails,
			Followings: followingDetails,
		},
	}, nil
}
func (s *SocialServiceService) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	currentTime := time.Now().UTC()
	collection := s.MongoClient.Database.Collection("users")

	objID, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid user ID format")
	}

	filter := bson.M{"_id": objID}
	update := bson.M{
		"$set": bson.M{
			"username":  req.UserName,
			"email":     req.Email,
			"tags":      req.Tags,
			"updatedAt": currentTime,
		},
	}

	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return &pb.UpdateUserResponse{
			Code: "COMM0501",
			Msg:  "Failed to update user",
		}, err
	}

	if result.MatchedCount == 0 {
		return &pb.UpdateUserResponse{
			Code: "COMM0201",
			Msg:  "User not found",
		}, nil
	}

	fmt.Println("Successfully created user")

	return &pb.UpdateUserResponse{
		Code: "COMM0000",
		Msg:  "User updated successfully",
	}, nil
}
func (s *SocialServiceService) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	collection := s.MongoClient.Database.Collection("users")

	objID, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid user ID format")
	}

	filter := bson.M{"_id": objID}
	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return &pb.DeleteUserResponse{
			Code: "COMM0501",
			Msg:  "Database error while deleting user",
		}, err
	}

	if result.DeletedCount == 0 {
		return &pb.DeleteUserResponse{
			Code: "COMM0300",
			Msg:  "User not found",
		}, nil
	}

	fmt.Println("Successfully deleted user")
	return &pb.DeleteUserResponse{
		Code: "COMM0003",
		Msg:  "User deleted successfully",
	}, nil
}
func (s *SocialServiceService) ListUsers(ctx context.Context, req *pb.ListUserRequest) (*pb.ListUserResponse, error) {
	collection := s.MongoClient.Database.Collection("users")

	// Pagination settings
	options := options.Find()
	options.SetLimit(int64(req.PageSize))
	options.SetSort(bson.M{"_id": 1}) // Sort in ascending order

	// Cursor-based pagination: If `NextCursor` exists, fetch records **after** it
	filter := bson.M{}
	if req.NextCursor != "" {
		cursorID, err := primitive.ObjectIDFromHex(req.NextCursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid cursor: %v", err)
		}
		filter["_id"] = bson.M{"$gt": cursorID} // Fetch users **after** this ID
	}

	// Fetch users
	cursor, err := collection.Find(ctx, filter, options)
	if err != nil {
		return &pb.ListUserResponse{
			Code: "COMM0501",
			Msg:  "Failed to fetch users",
		}, err
	}
	defer cursor.Close(ctx)

	var users []*pb.CreateUserResponse_UserData
	var lastUserID primitive.ObjectID

	for cursor.Next(ctx) {
		var user struct {
			ID        primitive.ObjectID   `bson:"_id"`
			UserName  string               `bson:"username"`
			Email     string               `bson:"email"`
			Tags      []string             `bson:"tags"`
			CreatedAt primitive.DateTime   `bson:"createdAt"`
			UpdatedAt primitive.DateTime   `bson:"updatedAt"`
			Followers []primitive.ObjectID `bson:"followers"`
			Following []primitive.ObjectID `bson:"following"`
		}

		if err := cursor.Decode(&user); err != nil {
			continue
		}

		// Store last user `_id` to generate `NextCursor`
		lastUserID = user.ID

		// Get followers' details
		followerDetails := []*pb.UserDetail{}
		if len(user.Followers) > 0 {
			followerFilter := bson.M{"_id": bson.M{"$in": user.Followers}}
			followerCursor, err := collection.Find(ctx, followerFilter)
			if err == nil {
				defer followerCursor.Close(ctx)
				for followerCursor.Next(ctx) {
					var follower struct {
						ID       primitive.ObjectID `bson:"_id"`
						UserName string             `bson:"username"`
					}
					if err := followerCursor.Decode(&follower); err == nil {
						followerDetails = append(followerDetails, &pb.UserDetail{
							Id:       strings.TrimSpace(follower.ID.Hex()),
							UserName: strings.TrimSpace(follower.UserName),
						})
					}
				}
			}
		}

		// Get following details
		followingDetails := []*pb.UserDetail{}
		if len(user.Following) > 0 {
			followingFilter := bson.M{"_id": bson.M{"$in": user.Following}}
			followingCursor, err := collection.Find(ctx, followingFilter)
			if err == nil {
				defer followingCursor.Close(ctx)
				for followingCursor.Next(ctx) {
					var following struct {
						ID       primitive.ObjectID `bson:"_id"`
						UserName string             `bson:"username"`
					}
					if err := followingCursor.Decode(&following); err == nil {
						followingDetails = append(followingDetails, &pb.UserDetail{
							Id:       strings.TrimSpace(following.ID.Hex()),
							UserName: strings.TrimSpace(following.UserName),
						})
					}
				}
			}
		}

		userData := &pb.CreateUserResponse_UserData{
			UserId:     user.ID.Hex(),
			UserName:   user.UserName,
			Email:      user.Email,
			Tags:       user.Tags,
			CreatedAt:  timestamppb.New(user.CreatedAt.Time()),
			UpdatedAt:  timestamppb.New(user.UpdatedAt.Time()),
			Followers:  followerDetails,
			Followings: followingDetails,
		}
		users = append(users, userData)
	}

	// Determine `NextCursor`
	nextCursor := ""
	if len(users) == int(req.PageSize) { // If full page, set next cursor
		nextCursor = lastUserID.Hex()
	}

	return &pb.ListUserResponse{
		Code: "COMM0000",
		Msg:  "Users retrieved successfully",
		Data: &pb.ListUserResponse_ListUsersData{
			Users:      users,
			NextCursor: nextCursor,
		},
	}, nil
}

func (s *SocialServiceService) FollowTipster(ctx context.Context, req *pb.FollowTipsterRequest) (*pb.FollowTipsterResponse, error) {
	tipsterId, err := primitive.ObjectIDFromHex(req.TipsterId)
	if err != nil {
		return &pb.FollowTipsterResponse{
			Code: "COMM0101",
			Msg:  "Invalid ID format",
		}, nil
	}
	userId, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.FollowTipsterResponse{
			Code: "COMM0101",
			Msg:  "Invalid tipster ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("users")

	// Update follower's following list
	userUpdate := bson.M{
		"$addToSet": bson.M{
			"following": tipsterId,
		},
	}

	// Update tipster's followers list
	tipsterUpdate := bson.M{
		"$addToSet": bson.M{
			"followers": userId,
		},
	}

	// Execute both updates in a transaction
	session, err := s.MongoClient.Database.Client().StartSession()
	if err != nil {
		return &pb.FollowTipsterResponse{
			Code: "COMM0501",
			Msg:  "Failed to start transaction",
		}, err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		_, err := collection.UpdateOne(sessCtx,
			bson.M{"_id": tipsterId},
			tipsterUpdate, nil)
		if err != nil {
			return nil, err
		}

		_, err = collection.UpdateOne(sessCtx,
			bson.M{"_id": userId},
			userUpdate, nil)
		return nil, err
	})

	if err != nil {
		return &pb.FollowTipsterResponse{
			Code: "COMM0501",
			Msg:  "Failed to update follow relationship",
		}, err
	}

	return &pb.FollowTipsterResponse{
		Code: "COMM0000",
		Msg:  "Successfully followed tipster",
		Data: &pb.FollowTipsterResponse_FollowData{
			IsFollowing: true,
			TipsterId:   req.TipsterId,
			UserId:      req.UserId,
		},
	}, nil
}
func (s *SocialServiceService) UnfollowTipster(ctx context.Context, req *pb.UnFollowTipsterRequest) (*pb.UnfollowTipsterResponse, error) {
	tipsterId, err := primitive.ObjectIDFromHex(req.TipsterId)
	if err != nil {
		return &pb.UnfollowTipsterResponse{
			Code: "COMM0101",
			Msg:  "Invalid ID format",
		}, nil
	}
	userId, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.UnfollowTipsterResponse{
			Code: "COMM0101",
			Msg:  "Invalid ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("users")

	// Update follower's following list
	userUpdate := bson.M{
		"$pull": bson.M{
			"following": tipsterId,
		},
	}

	// Update tipster's followers list
	tipsterUpdate := bson.M{
		"$pull": bson.M{
			"followers": userId,
		},
	}

	session, err := s.MongoClient.Database.Client().StartSession()
	if err != nil {
		return &pb.UnfollowTipsterResponse{
			Code: "COMM0501",
			Msg:  "Failed to start transaction",
		}, err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		_, err := collection.UpdateOne(sessCtx,
			bson.M{"_id": tipsterId},
			tipsterUpdate, nil)
		if err != nil {
			return nil, err
		}

		_, err = collection.UpdateOne(sessCtx,
			bson.M{"_id": userId},
			userUpdate, nil)
		return nil, err
	})

	if err != nil {
		return &pb.UnfollowTipsterResponse{
			Code: "COMM0501",
			Msg:  "Failed to update unfollow relationship",
		}, err
	}

	return &pb.UnfollowTipsterResponse{
		Code: "COMM0000",
		Msg:  "Successfully unfollowed tipster",
		Data: &pb.UnfollowTipsterResponse_UnfollowData{
			IsFollowing: false,
			TipsterId:   req.TipsterId,
			UserId:      req.UserId,
		},
	}, nil
}
func (s *SocialServiceService) CreateTip(ctx context.Context, req *pb.CreateTipRequest) (*pb.CreateTipResponse, error) {
	currentTime := time.Now().UTC()

	tipDoc := map[string]interface{}{
		"tipsterId": req.TipsterId,
		"title":     req.Title,
		"content":   req.Content,
		"tags":      req.Tags,
		"shareType": req.ShareType,
		"likes":     []primitive.ObjectID{},
		"unlikes":   []primitive.ObjectID{},
		"createdAt": currentTime,
		"updatedAt": currentTime,
	}

	collection := s.MongoClient.Database.Collection("tips")
	result, err := collection.InsertOne(ctx, tipDoc)
	if err != nil {
		return &pb.CreateTipResponse{
			Code: "COMM0501",
			Msg:  "Failed to create tip",
		}, err
	}

	return &pb.CreateTipResponse{
		Code: "COMM0000",
		Msg:  "Tip created successfully",
		Data: &pb.TipData{
			TipId:     result.InsertedID.(primitive.ObjectID).Hex(),
			TipsterId: req.TipsterId,
			Title:     req.Title,
			Content:   req.Content,
			Tags:      req.Tags,
			CreatedAt: timestamppb.New(currentTime),
			UpdatedAt: timestamppb.New(currentTime),
			ShareType: req.ShareType,
		},
	}, nil
}

func (s *SocialServiceService) GetTip(ctx context.Context, req *pb.GetTipRequest) (*pb.GetTipResponse, error) {
	if req.TipId == "" {
		return &pb.GetTipResponse{
			Code: "COMM0101",
			Msg:  "Tip ID is required",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("tips")
	usersCollection := s.MongoClient.Database.Collection("users")

	objID, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.GetTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	filter := bson.M{"_id": objID}

	var tip struct {
		ID        primitive.ObjectID   `bson:"_id"`
		TipsterID string               `bson:"tipsterId"`
		Title     string               `bson:"title"`
		Content   string               `bson:"content"`
		Tags      []string             `bson:"tags"`
		CreatedAt primitive.DateTime   `bson:"createdAt"`
		UpdatedAt primitive.DateTime   `bson:"updatedAt"`
		Likes     []primitive.ObjectID `bson:"likes"`
		Unlikes   []primitive.ObjectID `bson:"unlikes"`
		ShareType string               `bson:"shareType"`
	}

	err = collection.FindOne(ctx, filter).Decode(&tip)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, status.Errorf(codes.NotFound, "User not found")
		}
		return nil, status.Errorf(codes.Internal, "Error fetching user: %v", err)
	}

	// Get likes details
	var likesDetails []*pb.UserDetail
	if len(tip.Likes) > 0 {
		likesFilter := bson.M{"_id": bson.M{"$in": tip.Likes}}
		cursor, err := usersCollection.Find(ctx, likesFilter)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error fetching followers: %v", err)
		}
		defer cursor.Close(ctx)

		for cursor.Next(ctx) {
			var likes struct {
				ID       primitive.ObjectID `bson:"_id"`
				UserName string             `bson:"username"`
			}
			if err := cursor.Decode(&likes); err == nil {
				likesDetail := pb.UserDetail{
					Id:       strings.TrimSpace(likes.ID.Hex()), // Trim spaces
					UserName: strings.TrimSpace(likes.UserName),
				}
				likesDetails = append(likesDetails, &likesDetail)
			}
		}
	}

	// Get unlikes details
	var unLikesDetails []*pb.UserDetail
	if len(tip.Unlikes) > 0 {
		unLikesFilter := bson.M{"_id": bson.M{"$in": tip.Unlikes}}
		cursor, err := usersCollection.Find(ctx, unLikesFilter)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "Error fetching followers: %v", err)
		}
		defer cursor.Close(ctx)

		for cursor.Next(ctx) {
			var unlikes struct {
				ID       primitive.ObjectID `bson:"_id"`
				UserName string             `bson:"username"`
			}
			if err := cursor.Decode(&unlikes); err == nil {
				unLikesDetail := pb.UserDetail{
					Id:       strings.TrimSpace(unlikes.ID.Hex()), // Trim spaces
					UserName: strings.TrimSpace(unlikes.UserName),
				}
				unLikesDetails = append(unLikesDetails, &unLikesDetail)
			}
		}
	}

	return &pb.GetTipResponse{
		Code: "COMM0000",
		Msg:  "Tip fetched successfully",
		Data: &pb.TipData{
			TipId:     strings.TrimSpace(tip.ID.Hex()),
			TipsterId: tip.TipsterID,
			Title:     tip.Title,
			Content:   tip.Content,
			Tags:      tip.Tags,
			Likes:     likesDetails,
			Unlikes:   unLikesDetails,
			CreatedAt: timestamppb.New(tip.CreatedAt.Time()),
			UpdatedAt: timestamppb.New(tip.UpdatedAt.Time()),
			ShareType: tip.ShareType,
		},
	}, nil

}

func (s *SocialServiceService) UpdateTip(ctx context.Context, req *pb.UpdateTipRequest) (*pb.UpdateTipResponse, error) {
	tipId, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.UpdateTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	currentTime := time.Now().UTC()
	collection := s.MongoClient.Database.Collection("tips")

	update := bson.M{
		"$set": bson.M{
			"title":     req.Title,
			"content":   req.Content,
			"tags":      req.Tags,
			"shareType": req.ShareType,
			"updatedAt": currentTime,
		},
	}

	var tip map[string]interface{}
	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": tipId},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&tip)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &pb.UpdateTipResponse{
				Code: "COMM0300",
				Msg:  "Tip not found",
			}, nil
		}
		return &pb.UpdateTipResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	return &pb.UpdateTipResponse{
		Code: "COMM0000",
		Msg:  "Tip updated successfully",
	}, nil
}

func (s *SocialServiceService) DeleteTip(ctx context.Context, req *pb.DeleteTipRequest) (*pb.DeleteTipResponse, error) {
	tipId, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.DeleteTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("tips")
	result, err := collection.DeleteOne(ctx, bson.M{"_id": tipId})

	if err != nil {
		return &pb.DeleteTipResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	if result.DeletedCount == 0 {
		return &pb.DeleteTipResponse{
			Code: "COMM0300",
			Msg:  "Tip not found",
		}, nil
	}

	return &pb.DeleteTipResponse{
		Code: "COMM0003",
		Msg:  "Tip deleted successfully",
	}, nil
}

func (s *SocialServiceService) ListTips(ctx context.Context, req *pb.ListTipsRequest) (*pb.ListTipsResponse, error) {
	collection := s.MongoClient.Database.Collection("tips")
	usersCollection := s.MongoClient.Database.Collection("users")

	// Default page size
	pageSize := int64(req.PageSize)
	if pageSize <= 0 {
		pageSize = 10 // Default to 10 items per page
	}

	// Build filter
	filter := bson.M{}
	if req.TipsterId != "" {
		filter["tipsterId"] = req.TipsterId
	}

	// Cursor-based pagination: If `NextCursor` exists, fetch records **after** it
	if req.NextCursor != "" {
		cursorID, err := primitive.ObjectIDFromHex(req.NextCursor)
		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "Invalid cursor: %v", err)
		}
		filter["_id"] = bson.M{"$gt": cursorID} // Fetch tips **after** this ID
	}

	// Pagination options
	findOptions := options.Find()
	findOptions.SetLimit(pageSize)
	findOptions.SetSort(bson.M{"_id": 1}) // Sort in ascending order for cursor pagination

	// Execute MongoDB query
	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return &pb.ListTipsResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}
	defer cursor.Close(ctx)

	// Collect retrieved tips
	var tips []*pb.TipData
	var allLikeIDs, allUnlikeIDs []primitive.ObjectID
	var lastTipID primitive.ObjectID

	type tipRecord struct {
		ID        primitive.ObjectID   `bson:"_id"`
		TipsterID string               `bson:"tipsterId"`
		Title     string               `bson:"title"`
		Content   string               `bson:"content"`
		Tags      []string             `bson:"tags"`
		CreatedAt primitive.DateTime   `bson:"createdAt"`
		UpdatedAt primitive.DateTime   `bson:"updatedAt"`
		Likes     []primitive.ObjectID `bson:"likes"`
		Unlikes   []primitive.ObjectID `bson:"unlikes"`
		ShareType string               `bson:"shareType"`
	}

	var tempTips []tipRecord

	for cursor.Next(ctx) {
		var tip tipRecord
		if err := cursor.Decode(&tip); err != nil {
			continue
		}
		tempTips = append(tempTips, tip)

		// Store last `_id` to generate `NextCursor`
		lastTipID = tip.ID

		// Collect all like and unlike user IDs for batch query
		allLikeIDs = append(allLikeIDs, tip.Likes...)
		allUnlikeIDs = append(allUnlikeIDs, tip.Unlikes...)
	}

	// Fetch user details in bulk
	likeUserDetails := fetchUserDetails(ctx, usersCollection, allLikeIDs)
	unlikeUserDetails := fetchUserDetails(ctx, usersCollection, allUnlikeIDs)

	// Map user details for quick lookup
	likeUserMap := map[string]*pb.UserDetail{}
	for _, user := range likeUserDetails {
		likeUserMap[user.Id] = user
	}

	unlikeUserMap := map[string]*pb.UserDetail{}
	for _, user := range unlikeUserDetails {
		unlikeUserMap[user.Id] = user
	}

	// Convert raw tip records into TipData response
	for _, tip := range tempTips {
		likes := []*pb.UserDetail{}
		for _, likeID := range tip.Likes {
			if user, exists := likeUserMap[likeID.Hex()]; exists {
				likes = append(likes, user)
			}
		}

		unlikes := []*pb.UserDetail{}
		for _, unlikeID := range tip.Unlikes {
			if user, exists := unlikeUserMap[unlikeID.Hex()]; exists {
				unlikes = append(unlikes, user)
			}
		}

		tips = append(tips, &pb.TipData{
			TipId:     tip.ID.Hex(),
			TipsterId: tip.TipsterID,
			Title:     tip.Title,
			Content:   tip.Content,
			Tags:      tip.Tags,
			Likes:     likes,
			Unlikes:   unlikes,
			CreatedAt: timestamppb.New(tip.CreatedAt.Time()),
			UpdatedAt: timestamppb.New(tip.UpdatedAt.Time()),
			ShareType: tip.ShareType,
		})
	}

	// Determine `NextCursor`
	nextCursor := ""
	if len(tips) == int(pageSize) {
		nextCursor = lastTipID.Hex()
	}

	return &pb.ListTipsResponse{
		Code: "COMM0000",
		Msg:  "Tips retrieved successfully",
		Data: &pb.ListTipsResponse_ListTipsData{
			Tips:       tips,
			NextCursor: nextCursor,
		},
	}, nil
}

func fetchUserDetails(ctx context.Context, usersCollection *mongo.Collection, userIDs []primitive.ObjectID) []*pb.UserDetail {
	var userDetails []*pb.UserDetail
	if len(userIDs) == 0 {
		return userDetails
	}

	// Fetch users in bulk
	filter := bson.M{"_id": bson.M{"$in": userIDs}}
	cursor, err := usersCollection.Find(ctx, filter)
	if err != nil {
		return userDetails // Return empty list on error
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var user struct {
			ID       primitive.ObjectID `bson:"_id"`
			UserName string             `bson:"username"`
		}
		if err := cursor.Decode(&user); err == nil {
			userDetails = append(userDetails, &pb.UserDetail{
				Id:       user.ID.Hex(),
				UserName: strings.TrimSpace(user.UserName),
			})
		}
	}
	return userDetails
}

func (s *SocialServiceService) LikeTip(ctx context.Context, req *pb.LikeTipRequest) (*pb.LikeTipResponse, error) {
	if req.TipId == "" || req.UserId == "" {
		return &pb.LikeTipResponse{
			Code: "COMM0101",
			Msg:  "Tip ID and User ID are required",
		}, nil
	}

	// Convert to ObjectID
	tipId, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.LikeTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	userId, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.LikeTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid user ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("tips")

	// Check if user already liked the tip (since `likes` is an array of ObjectID)
	var existingTip struct {
		Likes []primitive.ObjectID `bson:"likes"`
	}

	err = collection.FindOne(ctx, bson.M{"_id": tipId, "likes": userId}).Decode(&existingTip)
	if err == nil {
		// User already liked this tip
		return &pb.LikeTipResponse{
			Code: "COMM0000",
			Msg:  "User already liked this tip",
			Data: &pb.LikeTipResponse_LikeTipData{
				TotalLikes: int32(len(existingTip.Likes)),
				UserLiked:  true,
			},
		}, nil
	}

	// Add userId to `likes` array and remove from `unlikes`
	update := bson.M{
		"$pull": bson.M{
			"unlikes": userId, // Remove user from `unlikes` if they previously disliked
		},
		"$addToSet": bson.M{
			"likes": userId, // Ensure userId is only added once
		},
	}

	// Get updated document
	var updatedTip struct {
		Likes []primitive.ObjectID `bson:"likes"`
	}

	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": tipId},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updatedTip)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &pb.LikeTipResponse{
				Code: "COMM0300",
				Msg:  "Tip not found",
			}, nil
		}
		return &pb.LikeTipResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	// Compute total likes safely
	totalLikes := int32(len(updatedTip.Likes))

	return &pb.LikeTipResponse{
		Code: "COMM0000",
		Msg:  "Tip liked successfully",
		Data: &pb.LikeTipResponse_LikeTipData{
			TotalLikes: totalLikes,
			UserLiked:  true,
		},
	}, nil
}

func (s *SocialServiceService) UnlikeTip(ctx context.Context, req *pb.UnlikeTipRequest) (*pb.UnlikeTipResponse, error) {
	if req.TipId == "" || req.UserId == "" {
		return &pb.UnlikeTipResponse{
			Code: "COMM0101",
			Msg:  "Tip ID and User ID are required",
		}, nil
	}

	// Convert to ObjectID
	tipId, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.UnlikeTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	userId, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.UnlikeTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid user ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("tips")

	// Check if user already unliked the tip
	var existingTip struct {
		Unlikes []primitive.ObjectID `bson:"unlikes"`
	}

	err = collection.FindOne(ctx, bson.M{"_id": tipId, "unlikes": userId}).Decode(&existingTip)
	if err == nil {
		// User already unliked this tip
		return &pb.UnlikeTipResponse{
			Code: "COMM0000",
			Msg:  "User already unliked this tip",
			Data: &pb.UnlikeTipResponse_UnLikeTipData{
				TotalUnLikes: int32(len(existingTip.Unlikes)),
				UserUnLiked:  true, // Should be true, since the user has already unliked
			},
		}, nil
	}

	// Add unlike and remove like if exists
	update := bson.M{
		"$pull": bson.M{
			"likes": userId, // Remove user from `likes` if they previously liked
		},
		"$addToSet": bson.M{
			"unlikes": userId, // Ensure userId is only added once
		},
	}

	// Get updated document
	var updatedTip struct {
		Unlikes []primitive.ObjectID `bson:"unlikes"`
	}

	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": tipId},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updatedTip)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &pb.UnlikeTipResponse{
				Code: "COMM0300",
				Msg:  "Tip not found",
			}, nil
		}
		return &pb.UnlikeTipResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	// Compute total unlikes safely
	totalUnlikes := int32(len(updatedTip.Unlikes))

	return &pb.UnlikeTipResponse{
		Code: "COMM0000",
		Msg:  "Tip unliked successfully",
		Data: &pb.UnlikeTipResponse_UnLikeTipData{
			TotalUnLikes: totalUnlikes,
			UserUnLiked:  true, // Set to true since the user just unliked
		},
	}, nil
}

func (s *SocialServiceService) ShareTip(ctx context.Context, req *pb.ShareTipRequest) (*pb.ShareTipResponse, error) {
	tipId, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.ShareTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	currentTime := time.Now().UTC()

	// Update tip's ShareType
	tipsCollection := s.MongoClient.Database.Collection("tips")
	_, err = tipsCollection.UpdateOne(
		ctx,
		bson.M{"_id": tipId},
		bson.M{
			"$set": bson.M{
				"shareType": req.ShareType,
				"updatedAt": currentTime,
			},
		},
	)
	if err != nil {
		return &pb.ShareTipResponse{
			Code: "COMM0501",
			Msg:  "Failed to update tip share type",
		}, err
	}

	return &pb.ShareTipResponse{
		Code: "COMM0000",
		Msg:  "Tip shared successfully",
	}, nil
}

func (s *SocialServiceService) CommentOnTip(ctx context.Context, req *pb.CommentOnTipRequest) (*pb.CommentOnTipResponse, error) {
	_, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.CommentOnTipResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	currentTime := time.Now().UTC()
	collection := s.MongoClient.Database.Collection("comments")

	// Generate new ObjectID for the comment
	newCommentId := primitive.NewObjectID()

	// If parentId is empty, use the new comment's ID as parentId
	parentId := req.ParentId
	if parentId == "" {
		parentId = newCommentId.Hex()
	}

	commentDoc := map[string]interface{}{
		"_id":       newCommentId,
		"tipId":     req.TipId,
		"userId":    req.UserId,
		"parentId":  parentId,
		"content":   req.Content,
		"likes":     []primitive.ObjectID{},
		"unlikes":   []primitive.ObjectID{},
		"createdAt": currentTime,
		"updatedAt": currentTime,
	}

	result, err := collection.InsertOne(ctx, commentDoc)
	if err != nil {
		return &pb.CommentOnTipResponse{
			Code: "COMM0501",
			Msg:  "Failed to create comment",
		}, err
	}

	return &pb.CommentOnTipResponse{
		Code: "COMM0000",
		Msg:  "Comment created successfully",
		Data: &pb.CommentInfo{
			CommentId: result.InsertedID.(primitive.ObjectID).Hex(),
			TipId:     req.TipId,
			UserId:    req.UserId,
			ParentId:  parentId,
			Content:   req.Content,
			CreatedAt: timestamppb.New(currentTime),
			UpdatedAt: timestamppb.New(currentTime),
			Likes:     []*pb.UserDetail{},
			Unlikes:   []*pb.UserDetail{},
		},
	}, nil
}
func (s *SocialServiceService) UpdateComment(ctx context.Context, req *pb.UpdateCommentRequest) (*pb.UpdateCommentResponse, error) {
	commentId, err := primitive.ObjectIDFromHex(req.CommentId)
	if err != nil {
		return &pb.UpdateCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid comment ID format",
		}, nil
	}

	currentTime := time.Now().UTC()
	collection := s.MongoClient.Database.Collection("comments")

	update := bson.M{
		"$set": bson.M{
			"content":   req.Content,
			"updatedAt": currentTime,
		},
	}

	var comment map[string]interface{}
	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": commentId},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&comment)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &pb.UpdateCommentResponse{
				Code: "COMM0300",
				Msg:  "Comment not found",
			}, nil
		}
		return &pb.UpdateCommentResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	return &pb.UpdateCommentResponse{
		Code: "COMM0000",
		Msg:  "Comment updated successfully",
	}, nil
}

func (s *SocialServiceService) DeleteComment(ctx context.Context, req *pb.DeleteCommentRequest) (*pb.DeleteCommentResponse, error) {
	// Convert the CommentId string to ObjectID
	commentId, err := primitive.ObjectIDFromHex(req.CommentId)
	collection := s.MongoClient.Database.Collection("comments")
	if err != nil {
		return &pb.DeleteCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid comment ID format",
		}, nil
	}

	result, err := collection.DeleteOne(ctx, bson.M{"_id": commentId})
	if err != nil {
		return &pb.DeleteCommentResponse{
			Code: "COMM0102",
			Msg:  "Failed to delete comment",
		}, nil
	}

	if result.DeletedCount == 0 {
		return &pb.DeleteCommentResponse{
			Code: "COMM0103",
			Msg:  "Comment not found",
		}, nil
	}

	return &pb.DeleteCommentResponse{
		Code: "COMM0000",
		Msg:  "Comment deleted successfully",
	}, nil
}

func (s *SocialServiceService) ListTipComments(ctx context.Context, req *pb.ListTipCommentsRequest) (*pb.ListTipCommentsResponse, error) {
	collection := s.MongoClient.Database.Collection("comments")
	usersCollection := s.MongoClient.Database.Collection("users")

	_, err := primitive.ObjectIDFromHex(req.TipId)
	if err != nil {
		return &pb.ListTipCommentsResponse{
			Code: "COMM0101",
			Msg:  "Invalid tip ID format",
		}, nil
	}

	filter := bson.M{"tipId": req.TipId}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return &pb.ListTipCommentsResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}
	defer cursor.Close(ctx)

	var comments []*pb.CommentInfo
	for cursor.Next(ctx) {
		var comment struct {
			ID        primitive.ObjectID   `bson:"_id"`
			TipID     string               `bson:"tipId"`
			UserID    string               `bson:"userId"`
			ParentID  string               `bson:"parentId"`
			Content   string               `bson:"content"`
			Likes     []primitive.ObjectID `bson:"likes"`
			Unlikes   []primitive.ObjectID `bson:"unlikes"`
			CreatedAt time.Time            `bson:"createdAt"`
			UpdatedAt time.Time            `bson:"updatedAt"`
		}

		if err := cursor.Decode(&comment); err != nil {
			continue
		}

		// Get likes details
		var likesDetails []*pb.UserDetail
		if len(comment.Likes) > 0 {
			likesFilter := bson.M{"_id": bson.M{"$in": comment.Likes}}
			likesCursor, err := usersCollection.Find(ctx, likesFilter)
			if err == nil {
				defer likesCursor.Close(ctx)
				for likesCursor.Next(ctx) {
					var user struct {
						ID       primitive.ObjectID `bson:"_id"`
						UserName string             `bson:"username"`
					}
					if err := likesCursor.Decode(&user); err == nil {
						likesDetails = append(likesDetails, &pb.UserDetail{
							Id:       strings.TrimSpace(user.ID.Hex()),
							UserName: strings.TrimSpace(user.UserName),
						})
					}
				}
			}
		}

		// Get unlikes details
		var unlikesDetails []*pb.UserDetail
		if len(comment.Unlikes) > 0 {
			unlikesFilter := bson.M{"_id": bson.M{"$in": comment.Unlikes}}
			unlikesCursor, err := usersCollection.Find(ctx, unlikesFilter)
			if err == nil {
				defer unlikesCursor.Close(ctx)
				for unlikesCursor.Next(ctx) {
					var user struct {
						ID       primitive.ObjectID `bson:"_id"`
						UserName string             `bson:"username"`
					}
					if err := unlikesCursor.Decode(&user); err == nil {
						unlikesDetails = append(unlikesDetails, &pb.UserDetail{
							Id:       strings.TrimSpace(user.ID.Hex()),
							UserName: strings.TrimSpace(user.UserName),
						})
					}
				}
			}
		}

		commentInfo := &pb.CommentInfo{
			CommentId: comment.ID.Hex(),
			TipId:     comment.TipID,
			UserId:    comment.UserID,
			ParentId:  comment.ParentID,
			Content:   comment.Content,
			CreatedAt: timestamppb.New(comment.CreatedAt),
			UpdatedAt: timestamppb.New(comment.UpdatedAt),
			Likes:     likesDetails,
			Unlikes:   unlikesDetails,
		}
		comments = append(comments, commentInfo)
	}

	fmt.Println(comments)
	return &pb.ListTipCommentsResponse{
		Code:     "COMM0000",
		Msg:      "Comments retrieved successfully",
		Comments: comments,
	}, nil
}

func (s *SocialServiceService) LikeComment(ctx context.Context, req *pb.LikeCommentRequest) (*pb.LikeCommentResponse, error) {
	if req.CommentId == "" || req.UserId == "" {
		return &pb.LikeCommentResponse{
			Code: "COMM0101",
			Msg:  "Comment ID is required",
		}, nil
	}

	commentId, err := primitive.ObjectIDFromHex(req.CommentId)
	if err != nil {
		return &pb.LikeCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid comment ID format",
		}, nil
	}

	userId, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.LikeCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid user ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("comments")

	// First check if user already liked
	var existingComment struct {
		Likes []string `json:"likes"`
	}
	err = collection.FindOne(ctx, bson.M{
		"_id":   commentId,
		"likes": userId,
	}).Decode(&existingComment)

	if err == nil {
		// User already liked this comment
		return &pb.LikeCommentResponse{
			Code: "COMM0000",
			Msg:  "User already liked this comment",
			Data: &pb.LikeCommentResponse_LikeCommentData{
				TotalLikes: int32(len(existingComment.Likes)),
				UserLiked:  true,
			},
		}, nil
	}

	update := bson.M{
		"$pull": bson.M{
			"unlikes": userId,
		},
		"$addToSet": bson.M{
			"likes": userId,
		},
	}

	var updatedComment struct {
		Likes []primitive.ObjectID `json:"likes"`
	}
	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": commentId},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updatedComment)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &pb.LikeCommentResponse{
				Code: "COMM0300",
				Msg:  "Comment not found",
			}, nil
		}
		return &pb.LikeCommentResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	return &pb.LikeCommentResponse{
		Code: "COMM0000",
		Msg:  "Comment liked successfully",
		Data: &pb.LikeCommentResponse_LikeCommentData{
			TotalLikes: int32(len(updatedComment.Likes)),
			UserLiked:  true,
		},
	}, nil
}
func (s *SocialServiceService) UnlikeComment(ctx context.Context, req *pb.UnlikeCommentRequest) (*pb.UnlikeCommentResponse, error) {
	if req.CommentId == "" || req.UserId == "" {
		return &pb.UnlikeCommentResponse{
			Code: "COMM0101",
			Msg:  "Comment ID and User ID are required",
		}, nil
	}

	commentId, err := primitive.ObjectIDFromHex(req.CommentId)
	if err != nil {
		return &pb.UnlikeCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid comment ID format",
		}, nil
	}

	userId, err := primitive.ObjectIDFromHex(req.UserId)
	if err != nil {
		return &pb.UnlikeCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid user ID format",
		}, nil
	}

	collection := s.MongoClient.Database.Collection("comments")

	// First check if user already unliked
	var existingComment struct {
		Unlikes []primitive.ObjectID `json:"unlikes"`
	}
	err = collection.FindOne(ctx, bson.M{
		"_id":     commentId,
		"unlikes": userId,
	}).Decode(&existingComment)

	if err == nil {
		// User already unliked this comment
		return &pb.UnlikeCommentResponse{
			Code: "COMM0000",
			Msg:  "User already unliked this comment",
			Data: &pb.UnlikeCommentResponse_UnlikeCommentData{
				TotalUnLikes: int32(len(existingComment.Unlikes)),
				UserUnLiked:  true,
			},
		}, nil
	}

	update := bson.M{
		"$pull": bson.M{
			"likes": userId,
		},
		"$addToSet": bson.M{
			"unlikes": userId,
		},
	}

	var updatedComment struct {
		Unlikes []primitive.ObjectID `json:"unlikes"`
	}
	err = collection.FindOneAndUpdate(
		ctx,
		bson.M{"_id": commentId},
		update,
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	).Decode(&updatedComment)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			return &pb.UnlikeCommentResponse{
				Code: "COMM0300",
				Msg:  "Comment not found",
			}, nil
		}
		return &pb.UnlikeCommentResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}

	return &pb.UnlikeCommentResponse{
		Code: "COMM0000",
		Msg:  "Comment unliked successfully",
		Data: &pb.UnlikeCommentResponse_UnlikeCommentData{
			TotalUnLikes: int32(len(updatedComment.Unlikes)),
			UserUnLiked:  true,
		},
	}, nil
}

func (s *SocialServiceService) ReplyComment(ctx context.Context, req *pb.ReplyCommentRequest) (*pb.ReplyCommentResponse, error) {
	_, err := primitive.ObjectIDFromHex(req.ParentCommentId)
	if err != nil {
		return &pb.ReplyCommentResponse{
			Code: "COMM0101",
			Msg:  "Invalid parent comment ID format",
		}, nil
	}

	currentTime := time.Now().UTC()
	collection := s.MongoClient.Database.Collection("comments")

	// Create new reply document
	replyDoc := map[string]interface{}{
		"userId":    req.UserId,
		"parentId":  req.ParentCommentId,
		"content":   req.Content,
		"likes":     []primitive.ObjectID{},
		"unlikes":   []primitive.ObjectID{},
		"createdAt": currentTime,
		"updatedAt": currentTime,
	}

	result, err := collection.InsertOne(ctx, replyDoc)
	if err != nil {
		return &pb.ReplyCommentResponse{
			Code: "COMM0501",
			Msg:  "Failed to create reply",
		}, err
	}

	return &pb.ReplyCommentResponse{
		Code: "COMM0000",
		Msg:  "Reply created successfully",
		Data: &pb.ReplyCommentResponse_ReplyData{
			ReplyId:         result.InsertedID.(primitive.ObjectID).Hex(),
			ParentCommentId: req.ParentCommentId,
			UserId:          req.UserId,
			Content:         req.Content,
			DateCreated:     timestamppb.New(currentTime),
		},
	}, nil
}
func (s *SocialServiceService) ListCommentReplies(ctx context.Context, req *pb.ListCommentRepliesRequest) (*pb.ListCommentRepliesResponse, error) {
	collection := s.MongoClient.Database.Collection("comments")
	usersCollection := s.MongoClient.Database.Collection("users")

	_, err := primitive.ObjectIDFromHex(req.ParentCommentId)
	if err != nil {
		return &pb.ListCommentRepliesResponse{
			Code: "COMM0101",
			Msg:  "Invalid parent comment ID format",
		}, nil
	}

	filter := bson.M{"parentId": req.ParentCommentId}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return &pb.ListCommentRepliesResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}
	defer cursor.Close(ctx)

	var replies []*pb.ReplyInfo
	for cursor.Next(ctx) {
		var reply struct {
			ID        primitive.ObjectID   `bson:"_id"`
			ParentID  string               `bson:"parentId"`
			UserID    string               `bson:"userId"`
			Content   string               `bson:"content"`
			Likes     []primitive.ObjectID `bson:"likes"`
			Unlikes   []primitive.ObjectID `bson:"unlikes"`
			CreatedAt time.Time            `bson:"createdAt"`
		}

		if err := cursor.Decode(&reply); err != nil {
			continue
		}

		// Get likes details
		var likesDetails []*pb.UserDetail
		if len(reply.Likes) > 0 {
			likesFilter := bson.M{"_id": bson.M{"$in": reply.Likes}}
			likesCursor, err := usersCollection.Find(ctx, likesFilter)
			if err == nil {
				defer likesCursor.Close(ctx)
				for likesCursor.Next(ctx) {
					var user struct {
						ID       primitive.ObjectID `bson:"_id"`
						UserName string             `bson:"username"`
					}
					if err := likesCursor.Decode(&user); err == nil {
						likesDetails = append(likesDetails, &pb.UserDetail{
							Id:       strings.TrimSpace(user.ID.Hex()),
							UserName: strings.TrimSpace(user.UserName),
						})
					}
				}
			}
		}

		// Get unlikes details
		var unlikesDetails []*pb.UserDetail
		if len(reply.Unlikes) > 0 {
			unlikesFilter := bson.M{"_id": bson.M{"$in": reply.Unlikes}}
			unlikesCursor, err := usersCollection.Find(ctx, unlikesFilter)
			if err == nil {
				defer unlikesCursor.Close(ctx)
				for unlikesCursor.Next(ctx) {
					var user struct {
						ID       primitive.ObjectID `bson:"_id"`
						UserName string             `bson:"username"`
					}
					if err := unlikesCursor.Decode(&user); err == nil {
						unlikesDetails = append(unlikesDetails, &pb.UserDetail{
							Id:       strings.TrimSpace(user.ID.Hex()),
							UserName: strings.TrimSpace(user.UserName),
						})
					}
				}
			}
		}

		if reply.ID.Hex() == req.ParentCommentId {
			continue
		}
		replyInfo := &pb.ReplyInfo{
			ReplyId:         reply.ID.Hex(),
			ParentCommentId: reply.ParentID,
			UserId:          reply.UserID,
			Content:         reply.Content,
			DateCreated:     timestamppb.New(reply.CreatedAt),
			Likes:           likesDetails,
			Unlikes:         unlikesDetails,
		}
		replies = append(replies, replyInfo)
	}

	return &pb.ListCommentRepliesResponse{
		Code:    "COMM0000",
		Msg:     "Replies retrieved successfully",
		Replies: replies,
	}, nil
}

func (s *SocialServiceService) ListComments(ctx context.Context, req *pb.ListCommentsRequest) (*pb.ListCommentsResponse, error) {
	collection := s.MongoClient.Database.Collection("comments")
	usersCollection := s.MongoClient.Database.Collection("users")

	pageSize := int64(req.PageSize)
	if pageSize <= 0 {
		pageSize = 10
	}

	pageNumber := 1
	if req.NextCursor != "" {
		if p, err := strconv.Atoi(req.NextCursor); err == nil && p > 0 {
			pageNumber = p
		}
	}

	findOptions := options.Find()
	findOptions.SetSkip((int64(pageNumber) - 1) * pageSize)
	findOptions.SetLimit(pageSize)
	findOptions.SetSort(bson.M{"createdAt": -1})

	cursor, err := collection.Find(ctx, bson.M{}, findOptions)
	if err != nil {
		return &pb.ListCommentsResponse{
			Code: "COMM0501",
			Msg:  "Database error",
		}, err
	}
	defer cursor.Close(ctx)

	var comments []*pb.CommentInfo
	var allLikeIDs, allUnlikeIDs []primitive.ObjectID

	type commentRecord struct {
		ID        primitive.ObjectID   `bson:"_id"`
		TipID     string               `bson:"tipId"`
		UserID    string               `bson:"userId"`
		ParentID  string               `bson:"parentId"`
		Content   string               `bson:"content"`
		CreatedAt time.Time            `bson:"createdAt"`
		UpdatedAt time.Time            `bson:"updatedAt"`
		Likes     []primitive.ObjectID `bson:"likes"`
		Unlikes   []primitive.ObjectID `bson:"unlikes"`
	}

	var tempComments []commentRecord

	for cursor.Next(ctx) {
		var comment commentRecord
		if err := cursor.Decode(&comment); err != nil {
			continue
		}
		tempComments = append(tempComments, comment)
		allLikeIDs = append(allLikeIDs, comment.Likes...)
		allUnlikeIDs = append(allUnlikeIDs, comment.Unlikes...)
	}

	likeUserDetails := fetchUserDetails(ctx, usersCollection, allLikeIDs)
	unlikeUserDetails := fetchUserDetails(ctx, usersCollection, allUnlikeIDs)

	likeUserMap := map[string]*pb.UserDetail{}
	for _, user := range likeUserDetails {
		likeUserMap[user.Id] = user
	}

	unlikeUserMap := map[string]*pb.UserDetail{}
	for _, user := range unlikeUserDetails {
		unlikeUserMap[user.Id] = user
	}

	for _, comment := range tempComments {
		likes := []*pb.UserDetail{}
		for _, likeID := range comment.Likes {
			if user, exists := likeUserMap[likeID.Hex()]; exists {
				likes = append(likes, user)
			}
		}

		unlikes := []*pb.UserDetail{}
		for _, unlikeID := range comment.Unlikes {
			if user, exists := unlikeUserMap[unlikeID.Hex()]; exists {
				unlikes = append(unlikes, user)
			}
		}

		comments = append(comments, &pb.CommentInfo{
			CommentId: comment.ID.Hex(),
			TipId:     comment.TipID,
			UserId:    comment.UserID,
			ParentId:  comment.ParentID,
			Content:   comment.Content,
			CreatedAt: timestamppb.New(comment.CreatedAt),
			UpdatedAt: timestamppb.New(comment.UpdatedAt),
			Likes:     likes,
			Unlikes:   unlikes,
		})
	}

	return &pb.ListCommentsResponse{
		Code:     "COMM0000",
		Msg:      "Comments retrieved successfully",
		Comments: comments,
	}, nil
}
func (s *SocialServiceService) ListFollowingFeed(ctx context.Context, req *pb.ListFollowingFeedRequest) (*pb.ListFollowingFeedResponse, error) {
	return &pb.ListFollowingFeedResponse{}, nil
}
