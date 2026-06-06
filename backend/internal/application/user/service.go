package user

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	domain "github.com/lgt/asr/internal/domain/user"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	"golang.org/x/crypto/bcrypt"
)

// usernamePattern restricts manually created usernames to Chinese characters,
// letters, digits, and underscores.
var usernamePattern = regexp.MustCompile(`^[\p{Han}A-Za-z0-9_]+$`)

// ErrInvalidCredentials means the username/password pair is not valid.
var ErrInvalidCredentials = errors.New("invalid credentials")

// Service orchestrates user use cases.
type Service struct {
	userRepo     domain.UserRepository
	workflowRepo wfdomain.WorkflowRepository
}

const (
	maxUsernameLength    = 64
	maxPasswordLength    = 128
	maxDisplayNameLength = 128
	deviceUsernamePrefix = "device_"

	defaultBatchWorkflowName    = "批量转写整理"
	defaultRealtimeWorkflowName = "实时转写整理"
	defaultMeetingWorkflowName  = "会议纪要工作流"
	defaultVoiceWorkflowName    = "语音控制工作流"
)

type ValidationError struct {
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}

func IsValidationError(err error) bool {
	var validationErr *ValidationError
	return errors.As(err, &validationErr)
}

// NewService creates a new user application service.
func NewService(userRepo domain.UserRepository, workflowRepo wfdomain.WorkflowRepository) *Service {
	return &Service{userRepo: userRepo, workflowRepo: workflowRepo}
}

// CreateUser registers a new user.
func (s *Service) CreateUser(ctx context.Context, req *CreateUserRequest) (*UserResponse, error) {
	if req == nil {
		return nil, &ValidationError{message: "username is required"}
	}
	username := strings.TrimSpace(req.Username)
	displayName := strings.TrimSpace(req.DisplayName)
	if username == "" {
		return nil, &ValidationError{message: "用户名长度范围为 1-64 个字符"}
	}
	if runeCount(username) > maxUsernameLength {
		return nil, &ValidationError{message: "用户名长度范围为 1-64 个字符"}
	}
	if !usernamePattern.MatchString(username) {
		return nil, &ValidationError{message: "用户名仅支持中文字母数字下划线"}
	}
	if req.Password == "" || runeCount(req.Password) > maxPasswordLength {
		return nil, &ValidationError{message: "密码长度范围为 1-128 个字符"}
	}
	if runeCount(displayName) > maxDisplayNameLength {
		return nil, &ValidationError{message: "display_name length must be at most 128 characters"}
	}

	role := domain.Role(req.Role)
	if role != domain.RoleAdmin && role != domain.RoleUser {
		return nil, errors.New("invalid role")
	}

	if _, err := s.userRepo.GetByUsername(ctx, username); err == nil {
		return nil, domain.ErrUserAlreadyExists
	} else if !errors.Is(err, domain.ErrUserNotFound) {
		return nil, err
	}

	hash, err := bcrypt.GenerateFromPassword(passwordBcryptInput(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Username:     username,
		PasswordHash: string(hash),
		DisplayName:  displayName,
		Role:         role,
	}

	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return toResponse(user), nil
}

func runeCount(value string) int {
	return len([]rune(value))
}

func passwordBcryptInput(password string) []byte {
	raw := []byte(password)
	if len(raw) <= 72 {
		return raw
	}
	sum := sha256.Sum256(raw)
	return []byte("sha256:" + base64.RawURLEncoding.EncodeToString(sum[:]))
}

func comparePassword(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err == nil {
		return nil
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), passwordBcryptInput(password))
}

// Authenticate verifies credentials and returns the user.
func (s *Service) Authenticate(ctx context.Context, username, password string) (*domain.User, error) {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, fmt.Errorf("load user by username: %w", err)
	}

	if err := comparePassword(user.PasswordHash, password); err != nil {
		return nil, ErrInvalidCredentials
	}

	return user, nil
}

// AuthenticateAnonymously issues or refreshes a user bound to a machine code.
func (s *Service) AuthenticateAnonymously(ctx context.Context, req *AnonymousLoginRequest) (*domain.User, error) {
	machineCode := strings.ToLower(strings.TrimSpace(req.MachineCode))
	if machineCode == "" {
		return nil, errors.New("machine_code is required")
	}
	normalizedIPAddresses := normalizeStringSlice(req.IPAddresses)
	normalizedMACAddresses := normalizeMACAddresses(req.MACAddresses)

	identity, err := s.userRepo.GetDeviceIdentityByMachineCode(ctx, machineCode)
	if err != nil && !errors.Is(err, domain.ErrDeviceIdentityNotFound) {
		return nil, err
	}
	if identity == nil && len(normalizedMACAddresses) > 0 {
		identity, err = s.userRepo.GetDeviceIdentityByMACAddresses(ctx, normalizedMACAddresses)
		if err != nil && !errors.Is(err, domain.ErrDeviceIdentityNotFound) {
			return nil, err
		}
	}

	var user *domain.User
	var identityID uint64
	if identity != nil {
		identityID = identity.ID
		user, err = s.userRepo.GetByID(ctx, identity.UserID)
		if err != nil {
			return nil, err
		}
	} else {
		user = &domain.User{
			Username:     buildDeviceUsername(machineCode),
			PasswordHash: "device-auth",
			DisplayName:  defaultDisplayName(req.DisplayName, req.Hostname, machineCode),
			Role:         domain.RoleUser,
		}
		if err := s.userRepo.Create(ctx, user); err != nil {
			return nil, err
		}
	}

	updatedDisplayName := strings.TrimSpace(req.DisplayName)
	if updatedDisplayName != "" && updatedDisplayName != user.DisplayName {
		user.DisplayName = updatedDisplayName
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, err
		}
	}

	deviceIdentity := &domain.DeviceIdentity{
		ID:           identityID,
		UserID:       user.ID,
		MachineCode:  machineCode,
		Hostname:     strings.TrimSpace(req.Hostname),
		Platform:     strings.TrimSpace(req.Platform),
		IPAddresses:  normalizedIPAddresses,
		MACAddresses: normalizedMACAddresses,
	}
	if err := s.userRepo.UpsertDeviceIdentity(ctx, deviceIdentity); err != nil {
		return nil, err
	}

	return user, nil
}

// GetUser retrieves user by ID.
func (s *Service) GetUser(ctx context.Context, id uint64) (*UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return toResponse(user), nil
}

// UpdateProfile updates the current user's display name.
func (s *Service) UpdateProfile(ctx context.Context, userID uint64, req *UpdateProfileRequest) (*UserResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	user.DisplayName = strings.TrimSpace(req.DisplayName)
	if user.DisplayName == "" {
		return nil, errors.New("display_name is required")
	}

	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}

	return toResponse(user), nil
}

// ListUsers returns a paginated list of users.
func (s *Service) ListUsers(ctx context.Context, offset, limit int) ([]*UserResponse, int64, error) {
	users, total, err := s.userRepo.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}

	items := make([]*UserResponse, len(users))
	for i, user := range users {
		items[i] = toResponse(user)
	}

	return items, total, nil
}

// GetWorkflowBindings returns current user's app workflow bindings.
func (s *Service) GetWorkflowBindings(ctx context.Context, userID uint64) (*WorkflowBindingsResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	bindings, err := s.userRepo.GetWorkflowBindings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.Role != domain.RoleAdmin {
		adminBindings, err := s.loadAdminWorkflowBindings(ctx, userID)
		if err != nil {
			return nil, err
		}
		bindings = mergeWorkflowBindings(bindings, adminBindings)
	}

	return toWorkflowBindingsResponse(bindings), nil
}

// UpdateWorkflowBindings validates and saves current user's app workflow bindings.
func (s *Service) UpdateWorkflowBindings(ctx context.Context, userID uint64, req *UpdateWorkflowBindingsRequest) (*WorkflowBindingsResponse, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if err := s.validateWorkflowBinding(ctx, user, "realtime", req.Realtime, wfdomain.WorkflowTypeRealtime); err != nil {
		return nil, err
	}
	if err := s.validateWorkflowBinding(ctx, user, "batch", req.Batch, wfdomain.WorkflowTypeBatch); err != nil {
		return nil, err
	}
	if err := s.validateWorkflowBinding(ctx, user, "meeting", req.Meeting, wfdomain.WorkflowTypeMeeting); err != nil {
		return nil, err
	}
	if err := s.validateWorkflowBinding(ctx, user, "voice_control", req.Voice, wfdomain.WorkflowTypeVoice); err != nil {
		return nil, err
	}

	bindings := &domain.WorkflowBindings{
		UserID:             userID,
		RealtimeWorkflowID: req.Realtime,
		BatchWorkflowID:    req.Batch,
		MeetingWorkflowID:  req.Meeting,
		VoiceWorkflowID:    req.Voice,
	}
	if err := s.userRepo.SaveWorkflowBindings(ctx, bindings); err != nil {
		return nil, err
	}

	return toWorkflowBindingsResponse(bindings), nil
}

// EnsureAdmin ensures the bootstrap admin account exists.
func (s *Service) EnsureAdmin(ctx context.Context, username, password, displayName string) error {
	user, err := s.userRepo.GetByUsername(ctx, username)
	if err == nil {
		if user.Role != domain.RoleAdmin {
			return errors.New("bootstrap username exists but is not admin")
		}
		return nil
	}
	if !errors.Is(err, domain.ErrUserNotFound) {
		return err
	}

	_, err = s.CreateUser(ctx, &CreateUserRequest{
		Username:    username,
		Password:    password,
		DisplayName: displayName,
		Role:        string(domain.RoleAdmin),
	})
	if errors.Is(err, domain.ErrUserAlreadyExists) {
		return nil
	}
	return err
}

// EnsureBuiltinWorkflowBindings fills empty bootstrap-admin workflow bindings
// with the built-in system workflow templates.
func (s *Service) EnsureBuiltinWorkflowBindings(ctx context.Context, username string) error {
	if s.workflowRepo == nil {
		return nil
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil
	}

	user, err := s.userRepo.GetByUsername(ctx, username)
	if err != nil {
		return err
	}
	if user.Role != domain.RoleAdmin {
		return errors.New("bootstrap username exists but is not admin")
	}

	bindings, err := s.userRepo.GetWorkflowBindings(ctx, user.ID)
	if err != nil {
		return err
	}
	if bindings == nil {
		bindings = &domain.WorkflowBindings{UserID: user.ID}
	}
	bindings.UserID = user.ID

	defaults, err := s.builtinDefaultWorkflowBindings(ctx)
	if err != nil {
		return err
	}

	changed := false
	changed = setMissingWorkflowBinding(&bindings.RealtimeWorkflowID, defaults.RealtimeWorkflowID) || changed
	changed = setMissingWorkflowBinding(&bindings.BatchWorkflowID, defaults.BatchWorkflowID) || changed
	changed = setMissingWorkflowBinding(&bindings.MeetingWorkflowID, defaults.MeetingWorkflowID) || changed
	changed = setMissingWorkflowBinding(&bindings.VoiceWorkflowID, defaults.VoiceWorkflowID) || changed
	if !changed {
		return nil
	}

	return s.userRepo.SaveWorkflowBindings(ctx, bindings)
}

func (s *Service) builtinDefaultWorkflowBindings(ctx context.Context) (*domain.WorkflowBindings, error) {
	sysType := wfdomain.OwnerSystem
	workflows, _, err := s.workflowRepo.List(ctx, &sysType, nil, true, 0, 0)
	if err != nil {
		return nil, err
	}

	var defaults domain.WorkflowBindings
	fallbacks := make(map[wfdomain.WorkflowType]uint64)
	for _, workflow := range workflows {
		if workflow == nil || workflow.ID == 0 {
			continue
		}
		switch strings.TrimSpace(workflow.Name) {
		case defaultRealtimeWorkflowName:
			if defaults.RealtimeWorkflowID == nil {
				defaults.RealtimeWorkflowID = uint64Ptr(workflow.ID)
			}
		case defaultBatchWorkflowName:
			if defaults.BatchWorkflowID == nil {
				defaults.BatchWorkflowID = uint64Ptr(workflow.ID)
			}
		case defaultMeetingWorkflowName:
			if defaults.MeetingWorkflowID == nil {
				defaults.MeetingWorkflowID = uint64Ptr(workflow.ID)
			}
		case defaultVoiceWorkflowName:
			if defaults.VoiceWorkflowID == nil {
				defaults.VoiceWorkflowID = uint64Ptr(workflow.ID)
			}
		}

		if workflow.IsLegacy || workflow.WorkflowType == "" {
			continue
		}
		if _, ok := fallbacks[workflow.WorkflowType]; !ok {
			fallbacks[workflow.WorkflowType] = workflow.ID
		}
	}

	if defaults.RealtimeWorkflowID == nil {
		defaults.RealtimeWorkflowID = uint64Ptr(fallbacks[wfdomain.WorkflowTypeRealtime])
	}
	if defaults.BatchWorkflowID == nil {
		defaults.BatchWorkflowID = uint64Ptr(fallbacks[wfdomain.WorkflowTypeBatch])
	}
	if defaults.MeetingWorkflowID == nil {
		defaults.MeetingWorkflowID = uint64Ptr(fallbacks[wfdomain.WorkflowTypeMeeting])
	}
	if defaults.VoiceWorkflowID == nil {
		defaults.VoiceWorkflowID = uint64Ptr(fallbacks[wfdomain.WorkflowTypeVoice])
	}

	return &defaults, nil
}

func setMissingWorkflowBinding(target **uint64, fallback *uint64) bool {
	if *target != nil || fallback == nil || *fallback == 0 {
		return false
	}
	value := *fallback
	*target = &value
	return true
}

func uint64Ptr(value uint64) *uint64 {
	if value == 0 {
		return nil
	}
	return &value
}

func loadAllUsers(ctx context.Context, repo domain.UserRepository) ([]*domain.User, error) {
	const pageSize = 100
	users := make([]*domain.User, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		page, total, err := repo.List(ctx, offset, pageSize)
		if err != nil {
			return nil, err
		}
		users = append(users, page...)
		if len(users) >= int(total) || len(page) == 0 {
			break
		}
	}
	return users, nil
}

func (s *Service) loadAdminWorkflowBindings(ctx context.Context, currentUserID uint64) (*domain.WorkflowBindings, error) {
	users, err := loadAllUsers(ctx, s.userRepo)
	if err != nil {
		return nil, err
	}

	var admin *domain.User
	for _, item := range users {
		if item == nil || item.Role != domain.RoleAdmin || item.ID == currentUserID {
			continue
		}
		if admin == nil || item.ID < admin.ID {
			admin = item
		}
	}
	if admin == nil {
		return nil, nil
	}

	return s.userRepo.GetWorkflowBindings(ctx, admin.ID)
}

func mergeWorkflowBindings(primary *domain.WorkflowBindings, inherited *domain.WorkflowBindings) *domain.WorkflowBindings {
	if primary == nil && inherited == nil {
		return &domain.WorkflowBindings{}
	}

	result := &domain.WorkflowBindings{}
	if primary != nil {
		*result = *primary
	}
	if inherited == nil {
		return result
	}

	if result.RealtimeWorkflowID == nil {
		result.RealtimeWorkflowID = inherited.RealtimeWorkflowID
	}
	if result.BatchWorkflowID == nil {
		result.BatchWorkflowID = inherited.BatchWorkflowID
	}
	if result.MeetingWorkflowID == nil {
		result.MeetingWorkflowID = inherited.MeetingWorkflowID
	}
	if result.VoiceWorkflowID == nil {
		result.VoiceWorkflowID = inherited.VoiceWorkflowID
	}
	return result
}

func toResponse(user *domain.User) *UserResponse {
	return &UserResponse{
		ID:          user.ID,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        string(user.Role),
	}
}

// ToUserResponse converts a domain user into the public response shape.
func ToUserResponse(user *domain.User) *UserResponse {
	return toResponse(user)
}

func toWorkflowBindingsResponse(bindings *domain.WorkflowBindings) *WorkflowBindingsResponse {
	if bindings == nil {
		return &WorkflowBindingsResponse{}
	}

	return &WorkflowBindingsResponse{
		Realtime: bindings.RealtimeWorkflowID,
		Batch:    bindings.BatchWorkflowID,
		Meeting:  bindings.MeetingWorkflowID,
		Voice:    bindings.VoiceWorkflowID,
	}
}

func (s *Service) validateWorkflowBinding(ctx context.Context, user *domain.User, bindingKey string, workflowID *uint64, expectedType wfdomain.WorkflowType) error {
	if workflowID == nil {
		return nil
	}
	if *workflowID == 0 {
		return fmt.Errorf("%s workflow id must be positive", bindingKey)
	}
	if s.workflowRepo == nil {
		return errors.New("workflow repository is not configured")
	}

	workflow, err := s.workflowRepo.GetByID(ctx, *workflowID)
	if err != nil {
		return fmt.Errorf("%s workflow #%d not found", bindingKey, *workflowID)
	}
	if workflow.IsLegacy {
		return fmt.Errorf("%s workflow #%d is legacy and cannot be bound", bindingKey, *workflowID)
	}
	if workflow.WorkflowType != expectedType {
		return fmt.Errorf("%s workflow #%d must be %s", bindingKey, *workflowID, expectedType)
	}
	if !canAccessWorkflow(user, workflow) {
		return fmt.Errorf("%s workflow #%d is not accessible", bindingKey, *workflowID)
	}

	return nil
}

func canAccessWorkflow(user *domain.User, workflow *wfdomain.Workflow) bool {
	if user == nil || workflow == nil {
		return false
	}
	if user.Role == domain.RoleAdmin {
		return true
	}

	switch workflow.OwnerType {
	case wfdomain.OwnerUser:
		return workflow.OwnerID == user.ID
	case wfdomain.OwnerSystem:
		return workflow.IsPublished
	default:
		return false
	}
}

func buildDeviceUsername(machineCode string) string {
	username := deviceUsernamePrefix + machineCode
	if runeCount(username) <= maxUsernameLength {
		return username
	}

	sum := sha256.Sum256([]byte(machineCode))
	return deviceUsernamePrefix + base64.RawURLEncoding.EncodeToString(sum[:])
}

func defaultDisplayName(displayName, hostname, machineCode string) string {
	if name := strings.TrimSpace(displayName); name != "" {
		return name
	}
	if name := strings.TrimSpace(hostname); name != "" {
		return name
	}
	if len(machineCode) > 8 {
		machineCode = machineCode[:8]
	}
	return "桌面设备-" + machineCode
}

func normalizeStringSlice(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		set[value] = struct{}{}
	}
	if len(set) == 0 {
		return []string{}
	}
	result := make([]string, 0, len(set))
	for item := range set {
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}

func normalizeMACAddresses(items []string) []string {
	normalized := normalizeStringSlice(items)
	for index, item := range normalized {
		normalized[index] = strings.ToLower(item)
	}
	sort.Strings(normalized)
	return normalized
}
