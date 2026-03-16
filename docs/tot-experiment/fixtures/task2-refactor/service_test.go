package service_test

import (
	"strings"
	"testing"

	"refactor/service"
	"refactor/types"
)

func TestCreateUserSendsWelcomeNotification(t *testing.T) {
	svc := service.NewUserOrderService()

	user, err := svc.CreateUser("alice@example.com", "Alice", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if user.ID == "" {
		t.Fatal("expected user ID to be set")
	}
	if user.Email != "alice@example.com" {
		t.Errorf("expected email alice@example.com, got %s", user.Email)
	}
	if user.Verified {
		t.Error("expected user to not be verified initially")
	}

	notifications := svc.GetNotifications()
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(notifications))
	}
	if notifications[0].To != "alice@example.com" {
		t.Errorf("expected notification to alice@example.com, got %s", notifications[0].To)
	}
	if !strings.Contains(notifications[0].Subject, "Welcome") {
		t.Errorf("expected welcome subject, got %s", notifications[0].Subject)
	}
}

func TestCreateOrderSendsConfirmation(t *testing.T) {
	svc := service.NewUserOrderService()

	user, err := svc.CreateUser("bob@example.com", "Bob", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	items := []types.OrderItem{
		{ProductID: "prod-1", Name: "Widget", Price: 10.00, Quantity: 2},
		{ProductID: "prod-2", Name: "Gadget", Price: 24.50, Quantity: 1},
	}

	order, err := svc.CreateOrder(user.ID, items)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}
	if order.Status != "active" {
		t.Errorf("expected status active, got %s", order.Status)
	}
	if order.UserID != user.ID {
		t.Errorf("expected userID %s, got %s", user.ID, order.UserID)
	}

	expectedTotal := 10.00*2 + 24.50*1
	if order.Total != expectedTotal {
		t.Errorf("expected total %.2f, got %.2f", expectedTotal, order.Total)
	}

	// Welcome notification + order confirmation
	notifications := svc.GetNotifications()
	if len(notifications) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(notifications))
	}
	orderNotif := notifications[1]
	if orderNotif.To != "bob@example.com" {
		t.Errorf("expected notification to bob@example.com, got %s", orderNotif.To)
	}
	if !strings.Contains(orderNotif.Subject, "confirmed") {
		t.Errorf("expected confirmation subject, got %s", orderNotif.Subject)
	}
}

func TestCancelOrderSendsNotice(t *testing.T) {
	svc := service.NewUserOrderService()

	user, err := svc.CreateUser("carol@example.com", "Carol", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	items := []types.OrderItem{
		{ProductID: "prod-1", Name: "Thing", Price: 15.00, Quantity: 1},
	}
	order, err := svc.CreateOrder(user.ID, items)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	err = svc.CancelOrder(order.ID, user.ID)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	cancelled, err := svc.GetOrder(order.ID)
	if err != nil {
		t.Fatalf("GetOrder failed: %v", err)
	}
	if cancelled.Status != "cancelled" {
		t.Errorf("expected status cancelled, got %s", cancelled.Status)
	}

	// Welcome + order confirmation + cancellation notice
	notifications := svc.GetNotifications()
	if len(notifications) != 3 {
		t.Fatalf("expected 3 notifications, got %d", len(notifications))
	}
	cancelNotif := notifications[2]
	if !strings.Contains(cancelNotif.Subject, "cancelled") {
		t.Errorf("expected cancellation subject, got %s", cancelNotif.Subject)
	}
}

func TestEmailValidationRejectsBadEmails(t *testing.T) {
	svc := service.NewUserOrderService()

	cases := []struct {
		email string
		want  string
	}{
		{"", "email is required"},
		{"noatsign", "email must contain @"},
		{"@missing.local", "email format is invalid"},
		{"missing@", "email format is invalid"},
		{"user@nodot", "email domain must contain a dot"},
	}

	for _, tc := range cases {
		err := svc.ValidateEmail(tc.email)
		if err == nil {
			t.Errorf("ValidateEmail(%q): expected error, got nil", tc.email)
			continue
		}
		if err.Error() != tc.want {
			t.Errorf("ValidateEmail(%q): expected %q, got %q", tc.email, tc.want, err.Error())
		}
	}

	// Valid email should pass
	if err := svc.ValidateEmail("good@example.com"); err != nil {
		t.Errorf("ValidateEmail(good@example.com): unexpected error: %v", err)
	}
}

func TestOrderValidationRejectsEmptyOrders(t *testing.T) {
	svc := service.NewUserOrderService()

	err := svc.ValidateOrder(nil)
	if err == nil {
		t.Fatal("expected error for nil items")
	}
	if err.Error() != "order must have at least one item" {
		t.Errorf("unexpected error: %v", err)
	}

	err = svc.ValidateOrder([]types.OrderItem{})
	if err == nil {
		t.Fatal("expected error for empty items")
	}

	// Item with missing product ID
	err = svc.ValidateOrder([]types.OrderItem{
		{ProductID: "", Name: "Bad", Price: 10, Quantity: 1},
	})
	if err == nil {
		t.Fatal("expected error for missing product ID")
	}

	// Negative price
	err = svc.ValidateOrder([]types.OrderItem{
		{ProductID: "p1", Name: "Bad", Price: -5, Quantity: 1},
	})
	if err == nil {
		t.Fatal("expected error for negative price")
	}

	// Zero quantity
	err = svc.ValidateOrder([]types.OrderItem{
		{ProductID: "p1", Name: "Bad", Price: 10, Quantity: 0},
	})
	if err == nil {
		t.Fatal("expected error for zero quantity")
	}
}

func TestPermissionCheck(t *testing.T) {
	svc := service.NewUserOrderService()

	admin, err := svc.CreateUser("admin@example.com", "Admin", "admin")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	regular, err := svc.CreateUser("regular@example.com", "Regular", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	other, err := svc.CreateUser("other@example.com", "Other", "user")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Admin has admin permission
	ok, err := svc.CheckUserPermission(admin.ID, regular.ID, "admin")
	if err != nil {
		t.Fatalf("CheckUserPermission failed: %v", err)
	}
	if !ok {
		t.Error("expected admin to have admin permission")
	}

	// Regular user does not have admin permission
	ok, err = svc.CheckUserPermission(regular.ID, admin.ID, "admin")
	if err != nil {
		t.Fatalf("CheckUserPermission failed: %v", err)
	}
	if ok {
		t.Error("expected regular user to not have admin permission")
	}

	// Owner check — user is owner of themselves
	ok, err = svc.CheckUserPermission(regular.ID, regular.ID, "owner")
	if err != nil {
		t.Fatalf("CheckUserPermission failed: %v", err)
	}
	if !ok {
		t.Error("expected user to be owner of themselves")
	}

	// Owner check — user is NOT owner of another
	ok, err = svc.CheckUserPermission(regular.ID, other.ID, "owner")
	if err != nil {
		t.Fatalf("CheckUserPermission failed: %v", err)
	}
	if ok {
		t.Error("expected user to not be owner of another user")
	}

	// admin_or_owner — admin can act on anyone
	ok, err = svc.CheckUserPermission(admin.ID, other.ID, "admin_or_owner")
	if err != nil {
		t.Fatalf("CheckUserPermission failed: %v", err)
	}
	if !ok {
		t.Error("expected admin to pass admin_or_owner check")
	}

	// admin_or_owner — non-admin, non-owner fails
	ok, err = svc.CheckUserPermission(regular.ID, other.ID, "admin_or_owner")
	if err != nil {
		t.Fatalf("CheckUserPermission failed: %v", err)
	}
	if ok {
		t.Error("expected non-admin non-owner to fail admin_or_owner check")
	}

	// CancelOrder permission: only owner or admin can cancel
	items := []types.OrderItem{
		{ProductID: "p1", Name: "X", Price: 10, Quantity: 1},
	}
	order, err := svc.CreateOrder(regular.ID, items)
	if err != nil {
		t.Fatalf("CreateOrder failed: %v", err)
	}

	// Other user (non-admin, non-owner) cannot cancel
	err = svc.CancelOrder(order.ID, other.ID)
	if err == nil {
		t.Fatal("expected permission denied error")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("expected permission denied, got: %v", err)
	}

	// Admin can cancel anyone's order
	err = svc.CancelOrder(order.ID, admin.ID)
	if err != nil {
		t.Fatalf("admin CancelOrder failed: %v", err)
	}
}

func TestCalculateTotalMultipleItems(t *testing.T) {
	svc := service.NewUserOrderService()

	items := []types.OrderItem{
		{ProductID: "p1", Name: "A", Price: 10.00, Quantity: 3},
		{ProductID: "p2", Name: "B", Price: 5.50, Quantity: 2},
		{ProductID: "p3", Name: "C", Price: 100.00, Quantity: 1},
	}

	total := svc.CalculateTotal(items)
	expected := 10.00*3 + 5.50*2 + 100.00*1
	if total != expected {
		t.Errorf("expected total %.2f, got %.2f", expected, total)
	}
}

func TestListOrdersByUserReturnsOnlyThatUsersOrders(t *testing.T) {
	svc := service.NewUserOrderService()

	u1, _ := svc.CreateUser("user1@example.com", "User1", "user")
	u2, _ := svc.CreateUser("user2@example.com", "User2", "user")

	items := []types.OrderItem{
		{ProductID: "p1", Name: "Item", Price: 10, Quantity: 1},
	}

	svc.CreateOrder(u1.ID, items)
	svc.CreateOrder(u1.ID, items)
	svc.CreateOrder(u2.ID, items)

	u1Orders, err := svc.ListOrdersByUser(u1.ID)
	if err != nil {
		t.Fatalf("ListOrdersByUser failed: %v", err)
	}
	if len(u1Orders) != 2 {
		t.Errorf("expected 2 orders for user1, got %d", len(u1Orders))
	}
	for _, o := range u1Orders {
		if o.UserID != u1.ID {
			t.Errorf("order %s belongs to %s, expected %s", o.ID, o.UserID, u1.ID)
		}
	}

	u2Orders, err := svc.ListOrdersByUser(u2.ID)
	if err != nil {
		t.Fatalf("ListOrdersByUser failed: %v", err)
	}
	if len(u2Orders) != 1 {
		t.Errorf("expected 1 order for user2, got %d", len(u2Orders))
	}
}

func TestDeleteUserWithActiveOrdersFails(t *testing.T) {
	svc := service.NewUserOrderService()

	user, _ := svc.CreateUser("del@example.com", "Del", "user")
	items := []types.OrderItem{
		{ProductID: "p1", Name: "Item", Price: 10, Quantity: 1},
	}
	svc.CreateOrder(user.ID, items)

	// Should fail — user has active order
	err := svc.DeleteUser(user.ID)
	if err == nil {
		t.Fatal("expected error when deleting user with active orders")
	}
	if !strings.Contains(err.Error(), "active orders") {
		t.Errorf("expected active orders error, got: %v", err)
	}

	// Verify user still exists
	u, err := svc.GetUser(user.ID)
	if err != nil {
		t.Fatalf("GetUser failed after failed delete: %v", err)
	}
	if u.ID != user.ID {
		t.Error("user should still exist after failed delete")
	}
}

func TestDeleteUserSucceedsWithNoActiveOrders(t *testing.T) {
	svc := service.NewUserOrderService()

	user, _ := svc.CreateUser("gone@example.com", "Gone", "user")

	// No orders — delete should succeed
	err := svc.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}

	// Verify user is gone
	_, err = svc.GetUser(user.ID)
	if err == nil {
		t.Fatal("expected error when getting deleted user")
	}
}

func TestDeleteUserSucceedsWithCancelledOrders(t *testing.T) {
	svc := service.NewUserOrderService()

	user, _ := svc.CreateUser("cancel@example.com", "Cancel", "user")
	items := []types.OrderItem{
		{ProductID: "p1", Name: "Item", Price: 10, Quantity: 1},
	}
	order, _ := svc.CreateOrder(user.ID, items)

	// Cancel the order first
	err := svc.CancelOrder(order.ID, user.ID)
	if err != nil {
		t.Fatalf("CancelOrder failed: %v", err)
	}

	// Now delete should succeed — no active orders
	err = svc.DeleteUser(user.ID)
	if err != nil {
		t.Fatalf("DeleteUser failed: %v", err)
	}
}

func TestVerifyEmail(t *testing.T) {
	svc := service.NewUserOrderService()

	user, _ := svc.CreateUser("verify@example.com", "Verify", "user")
	if user.Verified {
		t.Fatal("user should not be verified initially")
	}

	err := svc.VerifyEmail(user.ID)
	if err != nil {
		t.Fatalf("VerifyEmail failed: %v", err)
	}

	u, _ := svc.GetUser(user.ID)
	if !u.Verified {
		t.Error("user should be verified after VerifyEmail")
	}
}

func TestUpdateUser(t *testing.T) {
	svc := service.NewUserOrderService()

	user, _ := svc.CreateUser("update@example.com", "Original", "user")

	updated, err := svc.UpdateUser(user.ID, "NewName", "admin")
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}
	if updated.Name != "NewName" {
		t.Errorf("expected name NewName, got %s", updated.Name)
	}
	if updated.Role != "admin" {
		t.Errorf("expected role admin, got %s", updated.Role)
	}

	// Partial update — empty string means no change
	partial, err := svc.UpdateUser(user.ID, "", "superadmin")
	if err != nil {
		t.Fatalf("UpdateUser failed: %v", err)
	}
	if partial.Name != "NewName" {
		t.Errorf("expected name to remain NewName, got %s", partial.Name)
	}
	if partial.Role != "superadmin" {
		t.Errorf("expected role superadmin, got %s", partial.Role)
	}
}

func TestDuplicateEmailRejected(t *testing.T) {
	svc := service.NewUserOrderService()

	_, err := svc.CreateUser("dup@example.com", "First", "user")
	if err != nil {
		t.Fatalf("first CreateUser failed: %v", err)
	}

	_, err = svc.CreateUser("dup@example.com", "Second", "user")
	if err == nil {
		t.Fatal("expected error for duplicate email")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("expected already registered error, got: %v", err)
	}
}

func TestCancelAlreadyCancelledOrder(t *testing.T) {
	svc := service.NewUserOrderService()

	user, _ := svc.CreateUser("twice@example.com", "Twice", "user")
	items := []types.OrderItem{
		{ProductID: "p1", Name: "Item", Price: 10, Quantity: 1},
	}
	order, _ := svc.CreateOrder(user.ID, items)

	err := svc.CancelOrder(order.ID, user.ID)
	if err != nil {
		t.Fatalf("first CancelOrder failed: %v", err)
	}

	err = svc.CancelOrder(order.ID, user.ID)
	if err == nil {
		t.Fatal("expected error when cancelling already cancelled order")
	}
	if !strings.Contains(err.Error(), "already cancelled") {
		t.Errorf("expected already cancelled error, got: %v", err)
	}
}
