package beans

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlushCacheNoop(t *testing.T) {
	client := &Client{beansDir: "/nonexistent"}
	err := client.FlushCache(context.Background())
	require.NoError(t, err)
}

func TestCloseNoop(t *testing.T) {
	client := &Client{beansDir: "/nonexistent"}
	err := client.Close()
	require.NoError(t, err)
}

func TestBeansWithDepsResult_GetBean(t *testing.T) {
	t.Run("returns BeanWithDeps for existing bean", func(t *testing.T) {
		result := &BeansWithDepsResult{
			Beans: map[string]Bean{
				"bean-1": {ID: "bean-1", Title: "Test Bean", Status: StatusTodo},
			},
			Dependencies: map[string][]Dependency{
				"bean-1": {{BeanID: "bean-1", BlockedByID: "bean-2"}},
			},
			Dependents: map[string][]Dependent{
				"bean-1": {{BeanID: "bean-3", BlockerID: "bean-1", Type: "blocking"}},
			},
		}

		beanWithDeps := result.GetBean("bean-1")
		require.NotNil(t, beanWithDeps)
		require.Equal(t, "bean-1", beanWithDeps.ID)
		require.Equal(t, "Test Bean", beanWithDeps.Title)
		require.Len(t, beanWithDeps.Dependencies, 1)
		require.Len(t, beanWithDeps.Dependents, 1)
	})

	t.Run("returns nil for non-existing bean", func(t *testing.T) {
		result := &BeansWithDepsResult{
			Beans:        map[string]Bean{},
			Dependencies: make(map[string][]Dependency),
			Dependents:   make(map[string][]Dependent),
		}

		beanWithDeps := result.GetBean("nonexistent")
		require.Nil(t, beanWithDeps)
	})
}

func TestBeansWithDepsResult_GetBeanWithRelationships(t *testing.T) {
	result := &BeansWithDepsResult{
		Beans: map[string]Bean{
			"parent-1": {ID: "parent-1", Title: "Parent Issue", Status: StatusTodo},
			"child-1":  {ID: "child-1", Title: "Child 1", Status: StatusCompleted},
			"child-2":  {ID: "child-2", Title: "Child 2", Status: StatusCompleted},
		},
		Dependencies: map[string][]Dependency{},
		Dependents: map[string][]Dependent{
			"parent-1": {
				{BeanID: "child-1", BlockerID: "parent-1", Type: "parent-child", Status: StatusCompleted, Title: "Child 1"},
				{BeanID: "child-2", BlockerID: "parent-1", Type: "parent-child", Status: StatusCompleted, Title: "Child 2"},
			},
		},
	}

	parent := result.GetBean("parent-1")
	require.NotNil(t, parent)
	require.Equal(t, "parent-1", parent.ID)
	require.Len(t, parent.Dependents, 2)
}

func TestBeanWithDepsStruct(t *testing.T) {
	bean := &Bean{
		ID:     "test-bean",
		Title:  "Test Bean",
		Status: StatusTodo,
	}

	beanWithDeps := &BeanWithDeps{
		Bean: bean,
		Dependencies: []Dependency{
			{BeanID: "test-bean", BlockedByID: "dep-1"},
		},
		Dependents: []Dependent{
			{BeanID: "child-1", BlockerID: "test-bean", Type: "parent-child"},
		},
	}

	require.Equal(t, "test-bean", beanWithDeps.ID)
	require.Equal(t, "Test Bean", beanWithDeps.Title)
	require.Len(t, beanWithDeps.Dependencies, 1)
	require.Len(t, beanWithDeps.Dependents, 1)
}

func TestGetBeansWithDeps_EmptyInput(t *testing.T) {
	client := &Client{beansDir: "/nonexistent"}
	result, err := client.GetBeansWithDeps(context.Background(), []string{})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Beans)
}
