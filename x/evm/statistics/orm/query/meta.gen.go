// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.
// Code generated by gorm.io/gen. DO NOT EDIT.

package query

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"

	"gorm.io/gen"
	"gorm.io/gen/field"

	"gorm.io/plugin/dbresolver"

	"github.com/okex/exchain/x/evm/statistics/orm/model"
)

func newMetum(db *gorm.DB, opts ...gen.DOOption) metum {
	_metum := metum{}

	_metum.metumDo.UseDB(db, opts...)
	_metum.metumDo.UseModel(&model.Metum{})

	tableName := _metum.metumDo.TableName()
	_metum.ALL = field.NewAsterisk(tableName)
	_metum.ID = field.NewInt64(tableName, "id")
	_metum.Height = field.NewInt64(tableName, "height")

	_metum.fillFieldMap()

	return _metum
}

type metum struct {
	metumDo metumDo

	ALL    field.Asterisk
	ID     field.Int64
	Height field.Int64

	fieldMap map[string]field.Expr
}

func (m metum) Table(newTableName string) *metum {
	m.metumDo.UseTable(newTableName)
	return m.updateTableName(newTableName)
}

func (m metum) As(alias string) *metum {
	m.metumDo.DO = *(m.metumDo.As(alias).(*gen.DO))
	return m.updateTableName(alias)
}

func (m *metum) updateTableName(table string) *metum {
	m.ALL = field.NewAsterisk(table)
	m.ID = field.NewInt64(table, "id")
	m.Height = field.NewInt64(table, "height")

	m.fillFieldMap()

	return m
}

func (m *metum) WithContext(ctx context.Context) IMetumDo { return m.metumDo.WithContext(ctx) }

func (m metum) TableName() string { return m.metumDo.TableName() }

func (m metum) Alias() string { return m.metumDo.Alias() }

func (m *metum) GetFieldByName(fieldName string) (field.OrderExpr, bool) {
	_f, ok := m.fieldMap[fieldName]
	if !ok || _f == nil {
		return nil, false
	}
	_oe, ok := _f.(field.OrderExpr)
	return _oe, ok
}

func (m *metum) fillFieldMap() {
	m.fieldMap = make(map[string]field.Expr, 2)
	m.fieldMap["id"] = m.ID
	m.fieldMap["height"] = m.Height
}

func (m metum) clone(db *gorm.DB) metum {
	m.metumDo.ReplaceConnPool(db.Statement.ConnPool)
	return m
}

func (m metum) replaceDB(db *gorm.DB) metum {
	m.metumDo.ReplaceDB(db)
	return m
}

type metumDo struct{ gen.DO }

type IMetumDo interface {
	gen.SubQuery
	Debug() IMetumDo
	WithContext(ctx context.Context) IMetumDo
	WithResult(fc func(tx gen.Dao)) gen.ResultInfo
	ReplaceDB(db *gorm.DB)
	ReadDB() IMetumDo
	WriteDB() IMetumDo
	As(alias string) gen.Dao
	Session(config *gorm.Session) IMetumDo
	Columns(cols ...field.Expr) gen.Columns
	Clauses(conds ...clause.Expression) IMetumDo
	Not(conds ...gen.Condition) IMetumDo
	Or(conds ...gen.Condition) IMetumDo
	Select(conds ...field.Expr) IMetumDo
	Where(conds ...gen.Condition) IMetumDo
	Order(conds ...field.Expr) IMetumDo
	Distinct(cols ...field.Expr) IMetumDo
	Omit(cols ...field.Expr) IMetumDo
	Join(table schema.Tabler, on ...field.Expr) IMetumDo
	LeftJoin(table schema.Tabler, on ...field.Expr) IMetumDo
	RightJoin(table schema.Tabler, on ...field.Expr) IMetumDo
	Group(cols ...field.Expr) IMetumDo
	Having(conds ...gen.Condition) IMetumDo
	Limit(limit int) IMetumDo
	Offset(offset int) IMetumDo
	Count() (count int64, err error)
	Scopes(funcs ...func(gen.Dao) gen.Dao) IMetumDo
	Unscoped() IMetumDo
	Create(values ...*model.Metum) error
	CreateInBatches(values []*model.Metum, batchSize int) error
	Save(values ...*model.Metum) error
	First() (*model.Metum, error)
	Take() (*model.Metum, error)
	Last() (*model.Metum, error)
	Find() ([]*model.Metum, error)
	FindInBatch(batchSize int, fc func(tx gen.Dao, batch int) error) (results []*model.Metum, err error)
	FindInBatches(result *[]*model.Metum, batchSize int, fc func(tx gen.Dao, batch int) error) error
	Pluck(column field.Expr, dest interface{}) error
	Delete(...*model.Metum) (info gen.ResultInfo, err error)
	Update(column field.Expr, value interface{}) (info gen.ResultInfo, err error)
	UpdateSimple(columns ...field.AssignExpr) (info gen.ResultInfo, err error)
	Updates(value interface{}) (info gen.ResultInfo, err error)
	UpdateColumn(column field.Expr, value interface{}) (info gen.ResultInfo, err error)
	UpdateColumnSimple(columns ...field.AssignExpr) (info gen.ResultInfo, err error)
	UpdateColumns(value interface{}) (info gen.ResultInfo, err error)
	UpdateFrom(q gen.SubQuery) gen.Dao
	Attrs(attrs ...field.AssignExpr) IMetumDo
	Assign(attrs ...field.AssignExpr) IMetumDo
	Joins(fields ...field.RelationField) IMetumDo
	Preload(fields ...field.RelationField) IMetumDo
	FirstOrInit() (*model.Metum, error)
	FirstOrCreate() (*model.Metum, error)
	FindByPage(offset int, limit int) (result []*model.Metum, count int64, err error)
	ScanByPage(result interface{}, offset int, limit int) (count int64, err error)
	Scan(result interface{}) (err error)
	Returning(value interface{}, columns ...string) IMetumDo
	UnderlyingDB() *gorm.DB
	schema.Tabler
}

func (m metumDo) Debug() IMetumDo {
	return m.withDO(m.DO.Debug())
}

func (m metumDo) WithContext(ctx context.Context) IMetumDo {
	return m.withDO(m.DO.WithContext(ctx))
}

func (m metumDo) ReadDB() IMetumDo {
	return m.Clauses(dbresolver.Read)
}

func (m metumDo) WriteDB() IMetumDo {
	return m.Clauses(dbresolver.Write)
}

func (m metumDo) Session(config *gorm.Session) IMetumDo {
	return m.withDO(m.DO.Session(config))
}

func (m metumDo) Clauses(conds ...clause.Expression) IMetumDo {
	return m.withDO(m.DO.Clauses(conds...))
}

func (m metumDo) Returning(value interface{}, columns ...string) IMetumDo {
	return m.withDO(m.DO.Returning(value, columns...))
}

func (m metumDo) Not(conds ...gen.Condition) IMetumDo {
	return m.withDO(m.DO.Not(conds...))
}

func (m metumDo) Or(conds ...gen.Condition) IMetumDo {
	return m.withDO(m.DO.Or(conds...))
}

func (m metumDo) Select(conds ...field.Expr) IMetumDo {
	return m.withDO(m.DO.Select(conds...))
}

func (m metumDo) Where(conds ...gen.Condition) IMetumDo {
	return m.withDO(m.DO.Where(conds...))
}

func (m metumDo) Exists(subquery interface{ UnderlyingDB() *gorm.DB }) IMetumDo {
	return m.Where(field.CompareSubQuery(field.ExistsOp, nil, subquery.UnderlyingDB()))
}

func (m metumDo) Order(conds ...field.Expr) IMetumDo {
	return m.withDO(m.DO.Order(conds...))
}

func (m metumDo) Distinct(cols ...field.Expr) IMetumDo {
	return m.withDO(m.DO.Distinct(cols...))
}

func (m metumDo) Omit(cols ...field.Expr) IMetumDo {
	return m.withDO(m.DO.Omit(cols...))
}

func (m metumDo) Join(table schema.Tabler, on ...field.Expr) IMetumDo {
	return m.withDO(m.DO.Join(table, on...))
}

func (m metumDo) LeftJoin(table schema.Tabler, on ...field.Expr) IMetumDo {
	return m.withDO(m.DO.LeftJoin(table, on...))
}

func (m metumDo) RightJoin(table schema.Tabler, on ...field.Expr) IMetumDo {
	return m.withDO(m.DO.RightJoin(table, on...))
}

func (m metumDo) Group(cols ...field.Expr) IMetumDo {
	return m.withDO(m.DO.Group(cols...))
}

func (m metumDo) Having(conds ...gen.Condition) IMetumDo {
	return m.withDO(m.DO.Having(conds...))
}

func (m metumDo) Limit(limit int) IMetumDo {
	return m.withDO(m.DO.Limit(limit))
}

func (m metumDo) Offset(offset int) IMetumDo {
	return m.withDO(m.DO.Offset(offset))
}

func (m metumDo) Scopes(funcs ...func(gen.Dao) gen.Dao) IMetumDo {
	return m.withDO(m.DO.Scopes(funcs...))
}

func (m metumDo) Unscoped() IMetumDo {
	return m.withDO(m.DO.Unscoped())
}

func (m metumDo) Create(values ...*model.Metum) error {
	if len(values) == 0 {
		return nil
	}
	return m.DO.Create(values)
}

func (m metumDo) CreateInBatches(values []*model.Metum, batchSize int) error {
	return m.DO.CreateInBatches(values, batchSize)
}

// Save : !!! underlying implementation is different with GORM
// The method is equivalent to executing the statement: db.Clauses(clause.OnConflict{UpdateAll: true}).Create(values)
func (m metumDo) Save(values ...*model.Metum) error {
	if len(values) == 0 {
		return nil
	}
	return m.DO.Save(values)
}

func (m metumDo) First() (*model.Metum, error) {
	if result, err := m.DO.First(); err != nil {
		return nil, err
	} else {
		return result.(*model.Metum), nil
	}
}

func (m metumDo) Take() (*model.Metum, error) {
	if result, err := m.DO.Take(); err != nil {
		return nil, err
	} else {
		return result.(*model.Metum), nil
	}
}

func (m metumDo) Last() (*model.Metum, error) {
	if result, err := m.DO.Last(); err != nil {
		return nil, err
	} else {
		return result.(*model.Metum), nil
	}
}

func (m metumDo) Find() ([]*model.Metum, error) {
	result, err := m.DO.Find()
	return result.([]*model.Metum), err
}

func (m metumDo) FindInBatch(batchSize int, fc func(tx gen.Dao, batch int) error) (results []*model.Metum, err error) {
	buf := make([]*model.Metum, 0, batchSize)
	err = m.DO.FindInBatches(&buf, batchSize, func(tx gen.Dao, batch int) error {
		defer func() { results = append(results, buf...) }()
		return fc(tx, batch)
	})
	return results, err
}

func (m metumDo) FindInBatches(result *[]*model.Metum, batchSize int, fc func(tx gen.Dao, batch int) error) error {
	return m.DO.FindInBatches(result, batchSize, fc)
}

func (m metumDo) Attrs(attrs ...field.AssignExpr) IMetumDo {
	return m.withDO(m.DO.Attrs(attrs...))
}

func (m metumDo) Assign(attrs ...field.AssignExpr) IMetumDo {
	return m.withDO(m.DO.Assign(attrs...))
}

func (m metumDo) Joins(fields ...field.RelationField) IMetumDo {
	for _, _f := range fields {
		m = *m.withDO(m.DO.Joins(_f))
	}
	return &m
}

func (m metumDo) Preload(fields ...field.RelationField) IMetumDo {
	for _, _f := range fields {
		m = *m.withDO(m.DO.Preload(_f))
	}
	return &m
}

func (m metumDo) FirstOrInit() (*model.Metum, error) {
	if result, err := m.DO.FirstOrInit(); err != nil {
		return nil, err
	} else {
		return result.(*model.Metum), nil
	}
}

func (m metumDo) FirstOrCreate() (*model.Metum, error) {
	if result, err := m.DO.FirstOrCreate(); err != nil {
		return nil, err
	} else {
		return result.(*model.Metum), nil
	}
}

func (m metumDo) FindByPage(offset int, limit int) (result []*model.Metum, count int64, err error) {
	result, err = m.Offset(offset).Limit(limit).Find()
	if err != nil {
		return
	}

	if size := len(result); 0 < limit && 0 < size && size < limit {
		count = int64(size + offset)
		return
	}

	count, err = m.Offset(-1).Limit(-1).Count()
	return
}

func (m metumDo) ScanByPage(result interface{}, offset int, limit int) (count int64, err error) {
	count, err = m.Count()
	if err != nil {
		return
	}

	err = m.Offset(offset).Limit(limit).Scan(result)
	return
}

func (m metumDo) Scan(result interface{}) (err error) {
	return m.DO.Scan(result)
}

func (m metumDo) Delete(models ...*model.Metum) (result gen.ResultInfo, err error) {
	return m.DO.Delete(models)
}

func (m *metumDo) withDO(do gen.Dao) *metumDo {
	m.DO = *do.(*gen.DO)
	return m
}